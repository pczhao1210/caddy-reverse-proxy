package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	gatewayazure "github.com/aidockerfarm/gateway/internal/azure"
	appconfig "github.com/aidockerfarm/gateway/internal/config"
	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
)

type testCertificateStore struct {
	certificate model.CertificateConfig
}

type testSettingsStore struct {
	settings appconfig.Settings
	found    bool
}

type testAzurePermissionChecker struct {
	config model.AzureConfig
	result gatewayazure.PermissionCheckResult
	err    error
}

func (c *testAzurePermissionChecker) Check(_ context.Context, config model.AzureConfig) (gatewayazure.PermissionCheckResult, error) {
	c.config = config
	return c.result, c.err
}

func (s *testSettingsStore) Load() (appconfig.Settings, bool, error) {
	return s.settings, s.found, nil
}

func (s *testSettingsStore) Save(settings appconfig.Settings) error {
	s.settings = settings
	s.found = true
	return nil
}

type testDiscoverer struct {
	containers []model.ContainerService
	routes     []model.RouteConfig
}

func (d *testDiscoverer) Discover(context.Context) ([]model.ContainerService, []model.RouteConfig, error) {
	return d.containers, d.routes, nil
}

func (s *testCertificateStore) Save(certificate model.CertificateConfig) error {
	s.certificate = certificate
	return nil
}

type testCertificateReconciler struct {
	certificate model.CertificateConfig
	config      model.AppConfig
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

func (r *testCertificateReconciler) UpdateConfig(config model.AppConfig) {
	r.config = config
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
	handler := NewServer(Options{Config: model.AppConfig{Profile: model.ProfileVM}, Runtime: runtime}).Handler()

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

func TestStatusIncludesDeploymentMode(t *testing.T) {
	handler := NewServer(Options{
		Config:     model.AppConfig{DeploymentMode: model.ModeAzureVM},
		Store:      routes.NewStore(""),
		Reconciler: &testCertificateReconciler{},
	}).Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/status status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"deploymentMode":"azure-vm"`) {
		t.Fatalf("status response does not include deployment mode: %s", response.Body.String())
	}
}

func TestRoutingResourceAPILifecycle(t *testing.T) {
	store := routes.NewStore("")
	handler := NewServer(Options{Store: store, Reconciler: &testCertificateReconciler{}}).Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
		return recorder
	}

	response := request(http.MethodPost, "/api/listeners", `{"id":"listener-public","name":"Public HTTPS","hostname":"app.example.com","port":443,"protocol":"https"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /api/listeners status = %d, body = %s", response.Code, response.Body.String())
	}
	response = request(http.MethodPost, "/api/backend-pools", `{"id":"pool-app","name":"Application","targets":[{"address":"10.0.0.4"}]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /api/backend-pools status = %d, body = %s", response.Code, response.Body.String())
	}
	response = request(http.MethodPost, "/api/routing-rules", `{"id":"rule-api","name":"API","listenerId":"listener-public","backendPoolId":"pool-app","backendPort":8080,"backendProtocol":"http","pathPrefix":"/api","enabled":true}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /api/routing-rules status = %d, body = %s", response.Code, response.Body.String())
	}
	compiled := store.List()
	if len(compiled) != 1 || compiled[0].Host != "app.example.com" || compiled[0].Upstreams[0].URL != "http://10.0.0.4:8080" {
		t.Fatalf("compiled routes = %#v", compiled)
	}

	response = request(http.MethodDelete, "/api/listeners/listener-public", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("DELETE referenced listener status = %d, body = %s", response.Code, response.Body.String())
	}
	response = request(http.MethodDelete, "/api/backend-pools/pool-app", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("DELETE referenced backend pool status = %d, body = %s", response.Code, response.Body.String())
	}
	if response = request(http.MethodDelete, "/api/routing-rules/rule-api", ""); response.Code != http.StatusOK {
		t.Fatalf("DELETE routing rule status = %d, body = %s", response.Code, response.Body.String())
	}
	if response = request(http.MethodDelete, "/api/listeners/listener-public", ""); response.Code != http.StatusOK {
		t.Fatalf("DELETE listener status = %d, body = %s", response.Code, response.Body.String())
	}
	if response = request(http.MethodDelete, "/api/backend-pools/pool-app", ""); response.Code != http.StatusOK {
		t.Fatalf("DELETE backend pool status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSecuritySettingsPersistAndUpdateRuntimeConfig(t *testing.T) {
	config := settingsTestConfig()
	settingsStore := &testSettingsStore{settings: appconfig.SettingsFromConfig(config), found: true}
	reconciler := &testCertificateReconciler{}
	handler := NewServer(Options{Config: config, Reconciler: reconciler, SettingsStore: settingsStore}).Handler()
	body := `{"security":{"enabled":true,"maxRequestBodyBytes":2097152,"deniedMethods":["TRACE"],"deniedPathPrefixes":["/.git"],"allowedCidrs":[],"blockedCidrs":["203.0.113.0/24"]},"internalSourceRanges":["10.0.0.0/8"],"protectedRoutes":{"allowBearerToken":true,"allowAdminTokenHeader":false}}`
	request := authenticatedRequest(http.MethodPut, "/api/settings/security", body, "old-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings/security status = %d, body = %s", response.Code, response.Body.String())
	}
	if !settingsStore.found || settingsStore.settings.Security.MaxRequestBodyBytes != 2*1024*1024 {
		t.Fatalf("saved settings = %#v", settingsStore.settings)
	}
	if reconciler.config.Security.MaxRequestBodyBytes != 2*1024*1024 || reconciler.config.Auth.ProtectedRoutes.AllowAdminTokenHeader {
		t.Fatalf("runtime config = %#v", reconciler.config)
	}
}

func TestSecuritySettingsRejectNoProtectedTokenPolicy(t *testing.T) {
	config := settingsTestConfig()
	settingsStore := &testSettingsStore{settings: appconfig.SettingsFromConfig(config), found: true}
	handler := NewServer(Options{Config: config, Reconciler: &testCertificateReconciler{}, SettingsStore: settingsStore}).Handler()
	body := `{"security":{"enabled":true},"internalSourceRanges":["10.0.0.0/8"],"protectedRoutes":{"allowBearerToken":false,"allowAdminTokenHeader":false}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPut, "/api/settings/security", body, "old-token"))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "at least one token header policy") {
		t.Fatalf("PUT /api/settings/security status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAzureSettingsEqualTreatsNilAndEmptySlicesAsEqual(t *testing.T) {
	left := model.AzureConfig{ManageDNS: true, ManageNSG: true}
	right := model.AzureConfig{ManageDNS: true, ManageNSG: true, DNSZones: []model.AzureDNSZoneConfig{}, NSGSourceAddressPrefixes: []string{}}
	if !azureSettingsEqual(left, right) {
		t.Fatalf("azureSettingsEqual() = false for semantically equal configs")
	}
}

func TestAzurePermissionCheckUsesSubmittedSettings(t *testing.T) {
	checker := &testAzurePermissionChecker{result: gatewayazure.PermissionCheckResult{
		Credential: "DefaultAzureCredential",
		DNS: gatewayazure.PermissionGroupResult{
			Configured: true,
			Targets:    []gatewayazure.PermissionTargetResult{{Name: "new.example.com", ResourceGroup: "dns-rg", Granted: true}},
		},
	}}
	handler := NewServer(Options{AzurePermissionChecker: checker}).Handler()
	body := `{"azure":{"enabled":true,"manageDNS":true,"subscriptionId":"new-subscription","resourceGroup":"default-rg","dnsZones":[{"name":"new.example.com","resourceGroup":"dns-rg"}]}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/settings/azure/permissions", strings.NewReader(body)))

	if response.Code != http.StatusOK {
		t.Fatalf("POST /api/settings/azure/permissions status = %d, body = %s", response.Code, response.Body.String())
	}
	if checker.config.SubscriptionID != "new-subscription" || len(checker.config.DNSZones) != 1 || checker.config.DNSZones[0].ResourceGroup != "dns-rg" {
		t.Fatalf("checker config = %#v", checker.config)
	}
	if !strings.Contains(response.Body.String(), `"granted":true`) {
		t.Fatalf("permission response = %s", response.Body.String())
	}
}

func TestSystemSettingsRotateTokenAndStageDeploymentMode(t *testing.T) {
	config := settingsTestConfig()
	settingsStore := &testSettingsStore{settings: appconfig.SettingsFromConfig(config), found: true}
	reconciler := &testCertificateReconciler{}
	handler := NewServer(Options{Config: config, Reconciler: reconciler, SettingsStore: settingsStore}).Handler()
	body := `{"deploymentMode":"azure-vm","adminToken":"new-token","azure":{"enabled":false,"manageDNS":false,"manageNSG":false}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPut, "/api/settings/system", body, "old-token"))
	if response.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings/system status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"restartRequired":true`) {
		t.Fatalf("response does not require restart: %s", response.Body.String())
	}
	if settingsStore.settings.DeploymentMode != model.ModeAzureVM || settingsStore.settings.Auth.AdminToken != "new-token" {
		t.Fatalf("saved settings = %#v", settingsStore.settings)
	}
	if settingsStore.settings.DockerEnabled == nil || *settingsStore.settings.DockerEnabled {
		t.Fatalf("saved DockerEnabled = %#v, want false", settingsStore.settings.DockerEnabled)
	}

	oldTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(oldTokenResponse, authenticatedRequest(http.MethodGet, "/api/settings", "", "old-token"))
	if oldTokenResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want 401", oldTokenResponse.Code)
	}
	newTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(newTokenResponse, authenticatedRequest(http.MethodGet, "/api/settings", "", "new-token"))
	if newTokenResponse.Code != http.StatusOK {
		t.Fatalf("new token status = %d, body = %s", newTokenResponse.Code, newTokenResponse.Body.String())
	}
	if strings.Contains(newTokenResponse.Body.String(), "new-token") {
		t.Fatalf("settings response exposed admin token: %s", newTokenResponse.Body.String())
	}
}

func settingsTestConfig() model.AppConfig {
	return model.AppConfig{
		Profile:        model.ProfileVM,
		DeploymentMode: model.ModeContainerSocket,
		Control:        model.ControlConfig{Listen: ":8080"},
		Gateway: model.GatewayConfig{
			CaddyAdminEndpoint:   "http://127.0.0.1:2019",
			InternalSourceRanges: []string{"10.0.0.0/8"},
			Certificate:          model.CertificateConfig{Issuer: "letsencrypt"},
		},
		Auth: model.AuthConfig{
			Required:   true,
			AdminToken: "old-token",
			ProtectedRoutes: model.ProtectedRouteConfig{
				AllowBearerToken:      true,
				AllowAdminTokenHeader: true,
			},
		},
		Security: model.SecurityConfig{Enabled: true, MaxRequestBodyBytes: 10 * 1024 * 1024},
	}
}

func authenticatedRequest(method, path, body, token string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	return request
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

func TestWithoutGatewayContainerFiltersContainerAndRouteHint(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname() error = %v", err)
	}
	gatewayID := hostname + "abcdef"
	containers := []model.ContainerService{
		{ID: gatewayID, Name: "gateway"},
		{ID: "workload-id", Name: "workload"},
	}
	hints := []model.RouteConfig{
		{ID: "docker-" + shortID(gatewayID)},
		{ID: "docker-workload"},
	}

	visible, visibleHints := withoutGatewayContainer(containers, hints)
	if len(visible) != 1 || visible[0].ID != "workload-id" {
		t.Fatalf("visible containers = %#v, want only workload", visible)
	}
	if len(visibleHints) != 1 || visibleHints[0].ID != "docker-workload" {
		t.Fatalf("visible route hints = %#v, want only workload hint", visibleHints)
	}
}

func TestContainersEndpointExcludesGatewayContainer(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname() error = %v", err)
	}
	discoverer := &testDiscoverer{containers: []model.ContainerService{
		{ID: hostname + "abcdef", Name: "gateway", Networks: []string{"proxy"}, Ports: []model.ContainerPort{{PrivatePort: 8080, Type: "tcp"}}},
		{ID: "workload-id", Name: "workload", Networks: []string{"proxy"}, Ports: []model.ContainerPort{{PrivatePort: 8080, Type: "tcp"}}},
	}}
	handler := NewServer(Options{Discoverer: discoverer}).Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/discovery/containers", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/discovery/containers status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"name":"gateway"`) || !strings.Contains(response.Body.String(), `"name":"workload"`) {
		t.Fatalf("unexpected discovery response: %s", response.Body.String())
	}
}

func TestBindEndpointRejectsGatewayContainer(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname() error = %v", err)
	}
	gatewayID := hostname + "abcdef"
	discoverer := &testDiscoverer{containers: []model.ContainerService{{
		ID: gatewayID, Name: "gateway", Networks: []string{"proxy"}, Ports: []model.ContainerPort{{PrivatePort: 8080, Type: "tcp"}},
	}}}
	handler := NewServer(Options{Discoverer: discoverer}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/discovery/bind", strings.NewReader(`{"containerId":"`+gatewayID+`","port":8080}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "gateway container cannot be bound") {
		t.Fatalf("POST /api/discovery/bind status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestNormalizeUpstreamScheme(t *testing.T) {
	for _, test := range []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "", want: "http"},
		{input: " HTTPS ", want: "https"},
		{input: "tcp", wantErr: true},
	} {
		got, err := normalizeUpstreamScheme(test.input)
		if (err != nil) != test.wantErr {
			t.Fatalf("normalizeUpstreamScheme(%q) error = %v, wantErr %t", test.input, err, test.wantErr)
		}
		if got != test.want {
			t.Fatalf("normalizeUpstreamScheme(%q) = %q, want %q", test.input, got, test.want)
		}
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
