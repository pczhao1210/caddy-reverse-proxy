package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

type testCertificateStore struct {
	certificate model.CertificateConfig
}

func (s *testCertificateStore) Save(certificate model.CertificateConfig) error {
	s.certificate = certificate
	return nil
}

type testCertificateReconciler struct {
	certificate model.CertificateConfig
}

func (r *testCertificateReconciler) Sync(context.Context) model.ReconcileResult {
	return model.ReconcileResult{CaddyLoaded: true}
}

func (r *testCertificateReconciler) Last() model.ReconcileResult {
	return model.ReconcileResult{}
}

func (r *testCertificateReconciler) UpdateCertificate(certificate model.CertificateConfig) {
	r.certificate = certificate
}

type testRuntimeStatus struct {
	ready bool
	err   error
}

func (s *testRuntimeStatus) Ready() bool {
	return s.ready
}

func (s *testRuntimeStatus) LastError() error {
	return s.err
}

func TestHealthEndpointsReflectRuntimeReadiness(t *testing.T) {
	runtime := &testRuntimeStatus{err: errors.New("caddy exited")}
	handler := NewServer(Options{Config: model.AppConfig{Profile: model.ProfileACI}, Runtime: runtime}).Handler()

	assertStatus := func(path string, want int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("GET %s status = %d, want %d; body = %s", path, response.Code, want, response.Body.String())
		}
	}

	assertStatus("/livez", http.StatusOK)
	assertStatus("/readyz", http.StatusServiceUnavailable)
	assertStatus("/healthz", http.StatusServiceUnavailable)

	runtime.ready = true
	runtime.err = nil
	assertStatus("/readyz", http.StatusOK)
}

func TestContainerBindPolicySharedNetworkAllowsBind(t *testing.T) {
	container := model.ContainerService{
		Name:     "app",
		Networks: []string{"bridge", "proxy-net"},
		Ports:    []model.ContainerPort{{PrivatePort: 8080, Type: "tcp"}},
		NetworkEndpoints: []model.NetworkEndpoint{
			{Name: "bridge", Address: "172.17.0.5"},
			{Name: "proxy-net", Address: "172.22.0.5"},
		},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 8080)
	if !policy.CanBind {
		t.Fatalf("CanBind = false, want true")
	}
	if policy.Mode != "shared-network" {
		t.Fatalf("Mode = %q, want shared-network", policy.Mode)
	}
	if upstream := bindUpstreamName(container, []string{"proxy-net"}); upstream != "172.22.0.5" {
		t.Fatalf("bindUpstreamName = %q, want 172.22.0.5", upstream)
	}
}

func TestContainerBindPolicyHostNetworkSuggestsExplicitRoute(t *testing.T) {
	container := model.ContainerService{
		Name:     "portainer",
		Networks: []string{"host"},
		Ports:    []model.ContainerPort{{PrivatePort: 9443, Type: "tcp"}},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 9443)
	if policy.CanBind {
		t.Fatalf("CanBind = true, want false")
	}
	if policy.Mode != "host-network" {
		t.Fatalf("Mode = %q, want host-network", policy.Mode)
	}
	if policy.SuggestedUpstream != "http://host.docker.internal:9443" {
		t.Fatalf("SuggestedUpstream = %q", policy.SuggestedUpstream)
	}
}

func TestContainerBindPolicyBridgeWithoutSharedNetworkRequiresAttach(t *testing.T) {
	container := model.ContainerService{
		Name:             "bridge-only",
		Networks:         []string{"bridge"},
		Ports:            []model.ContainerPort{{PrivatePort: 8080, Type: "tcp"}},
		NetworkEndpoints: []model.NetworkEndpoint{{Name: "bridge", Address: "172.17.0.8"}},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 8080)
	if policy.CanBind {
		t.Fatalf("CanBind = true, want false")
	}
	if policy.Mode != "bridge-unreachable" {
		t.Fatalf("Mode = %q, want bridge-unreachable", policy.Mode)
	}
}

func TestContainerBindPolicyPublishedPortSuggestsHostGateway(t *testing.T) {
	container := model.ContainerService{
		Name:     "published-app",
		Networks: []string{"bridge"},
		Ports:    []model.ContainerPort{{PrivatePort: 8080, PublicPort: 18080, Type: "tcp"}},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 8080)
	if policy.Mode != "published-port" {
		t.Fatalf("Mode = %q, want published-port", policy.Mode)
	}
	if policy.SuggestedUpstream != "http://host.docker.internal:18080" {
		t.Fatalf("SuggestedUpstream = %q", policy.SuggestedUpstream)
	}
}

func TestCertificateUpdatePreservesAndDoesNotReturnClientSecret(t *testing.T) {
	store := &testCertificateStore{}
	reconciler := &testCertificateReconciler{}
	config := model.AppConfig{Gateway: model.GatewayConfig{Certificate: model.CertificateConfig{
		Issuer: "letsencrypt",
		DNSChallenge: model.DNSChallengeConfig{Provider: "azure", Azure: model.AzureDNSChallengeConfig{
			Authentication: "appregistration", ClientSecret: "existing-secret",
		}},
	}}}
	handler := NewServer(Options{Config: config, Reconciler: reconciler, CertificateStore: store}).Handler()
	body := []byte(`{"issuer":"letsencrypt","subjects":["*.example.com"],"dnsChallenge":{"provider":"azure","azure":{"subscriptionId":"subscription","resourceGroup":"dns-rg","authentication":"appRegistration","tenantId":"tenant","clientId":"client"}}}`)
	request := httptest.NewRequest(http.MethodPut, "/api/certificate", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT /api/certificate status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := store.certificate.DNSChallenge.Azure.ClientSecret; got != "existing-secret" {
		t.Fatalf("saved client secret = %q, want preserved secret", got)
	}
	if got := reconciler.certificate.DNSChallenge.Azure.ClientSecret; got != "existing-secret" {
		t.Fatalf("applied client secret = %q, want preserved secret", got)
	}
	if strings.Contains(response.Body.String(), "existing-secret") || strings.Contains(response.Body.String(), "clientSecret\"") {
		t.Fatalf("response exposed client secret: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"clientSecretConfigured":true`) {
		t.Fatalf("response did not report configured secret: %s", response.Body.String())
	}
}
