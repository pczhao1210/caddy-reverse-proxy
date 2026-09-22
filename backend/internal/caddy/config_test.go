package caddy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aidockerfarm/gateway/internal/auth"
	certificates "github.com/aidockerfarm/gateway/internal/certificate"
	"github.com/aidockerfarm/gateway/internal/model"
)

func TestRenderSeparatesHTTPAndHTTPSListeners(t *testing.T) {
	data, err := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{HTTPListen: ":80", HTTPSListen: ":443"}}).Render([]model.RouteConfig{
		{ID: "http", Host: "app.example.com", Enabled: true, ListenerPort: 8088, ListenerProtocol: "http", Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}},
		{ID: "https", Host: "secure.example.com", Enabled: true, HTTPS: true, ListenerPort: 8443, ListenerProtocol: "https", Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	servers := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)
	found := false
	for _, value := range servers {
		server := value.(map[string]any)
		for _, address := range server["listen"].([]any) {
			if address != ":8088" {
				continue
			}
			found = true
			if len(server["listen"].([]any)) != 1 {
				t.Fatal("HTTP listener shares a server with other endpoints")
			}
			if automatic, ok := server["automatic_https"].(map[string]any); !ok || automatic["disable"] != true {
				t.Fatal("HTTP listener must explicitly disable automatic HTTPS")
			}
		}
	}
	if !found {
		t.Fatal("custom HTTP listener is missing")
	}
}

func TestRuntimeMixedListeners(t *testing.T) {
	binary := os.Getenv("CADDY_TEST_BIN")
	if binary == "" {
		t.Skip("set CADDY_TEST_BIN to run real Caddy tests")
	}
	authConfig := model.AuthConfig{Required: true, AdminToken: "runtime-test-token"}
	backend := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Host == "protected.example.test" && (request.Header.Get("Authorization") != "" || request.Header.Get("X-Admin-Token") != "") {
			http.Error(writer, "gateway credentials leaked", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(writer, "upstream-ok")
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Host == "admin.example.test" && strings.HasPrefix(request.URL.Path, "/api/") {
			auth.Middleware(authConfig, backend).ServeHTTP(writer, request)
			return
		}
		backend.ServeHTTP(writer, request)
	}))
	defer upstream.Close()
	httpAddress, _ := testListenerAddress(t)
	httpsAddress, httpsPort := testListenerAddress(t)
	customAddress, customPort := testListenerAddress(t)
	adminAddress, _ := testListenerAddress(t)
	directory := t.TempDir()
	data, err := NewRenderer(model.AppConfig{Auth: authConfig, Control: model.ControlConfig{Listen: strings.TrimPrefix(upstream.URL, "http://"), ManagementHost: "admin.example.test"}, Gateway: model.GatewayConfig{
		HTTPListen: httpAddress, HTTPSListen: httpsAddress, CaddyAdminEndpoint: "http://" + adminAddress, CaddyDataDir: directory,
	}}).Render([]model.RouteConfig{
		{ID: "http", Host: "app.example.test", Enabled: true, ListenerPort: customPort, ListenerProtocol: "http", Upstreams: []model.UpstreamTarget{{URL: upstream.URL}}},
		{ID: "https", Host: "app.example.test", Enabled: true, HTTPS: true, ListenerPort: httpsPort, ListenerProtocol: "https", Upstreams: []model.UpstreamTarget{{URL: upstream.URL}}},
		{ID: "protected", Source: "management", Host: "protected.example.test", Enabled: true, HTTPS: true, Protected: true, Upstreams: []model.UpstreamTarget{{URL: upstream.URL}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	certificateServer := httptest.NewTLSServer(nil)
	certificate := certificateServer.TLS.Certificates[0]
	certificateServer.Close()
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	config["apps"].(map[string]any)["tls"] = map[string]any{"certificates": map[string]any{"load_pem": []any{map[string]any{"certificate": string(certificatePEM), "key": string(keyPEM)}}}}
	renderedServer(t, config, httpsAddress)["automatic_https"].(map[string]any)["disable_certificates"] = true
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	startTestCaddy(t, binary, data, adminAddress)
	active, err := NewClient("http://" + adminAddress).Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := certificates.ParsePolicy(active, directory); err == nil || !strings.Contains(err.Error(), "manual certificate loaders") {
		t.Fatalf("manual runtime certificate must not be archivable: %v", err)
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for _, endpoint := range []string{"http://" + customAddress, "https://" + httpsAddress} {
		request, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "app.example.test"
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || string(body) != "upstream-ok" {
			t.Fatalf("%s: status=%d body=%q err=%v", endpoint, response.StatusCode, body, err)
		}
	}
	request, _ := http.NewRequest(http.MethodGet, "http://"+httpAddress+"/hello", nil)
	request.Host = "app.example.test"
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusPermanentRedirect || !strings.Contains(response.Header.Get("Location"), ":"+strconv.Itoa(httpsPort)+"/hello") {
		t.Fatalf("redirect: status=%d location=%s", response.StatusCode, response.Header.Get("Location"))
	}
	for _, test := range []struct {
		host, path, header, token string
		status                    int
	}{
		{host: "admin.example.test", path: "/", status: 200},
		{host: "admin.example.test", path: "/api/status", status: 401},
		{host: "admin.example.test", path: "/api/status", header: "Authorization", token: "Bearer invalid", status: 401},
		{host: "admin.example.test", path: "/api/status", header: "Authorization", token: "Bearer " + authConfig.AdminToken, status: 200},
		{host: "admin.example.test", path: "/api/status", header: "X-Admin-Token", token: authConfig.AdminToken, status: 200},
		{host: "protected.example.test", path: "/", status: 401},
		{host: "protected.example.test", path: "/", header: "Authorization", token: "Bearer " + authConfig.AdminToken, status: 200},
	} {
		request, _ := http.NewRequest(http.MethodGet, "https://"+httpsAddress+test.path, nil)
		request.Host = test.host
		if test.header != "" {
			request.Header.Set(test.header, test.token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Errorf("%s%s (%s): got %d, want %d", test.host, test.path, test.header, response.StatusCode, test.status)
		}
	}
}

func testListenerAddress(t *testing.T) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String(), listener.Addr().(*net.TCPAddr).Port
}

func TestRenderedWildcardCertificateInventory(t *testing.T) {
	for _, explicitHost := range []bool{false, true} {
		subjects := []string{"*.example.com"}
		if explicitHost {
			subjects = append(subjects, "app.example.com")
		}
		data, err := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
			HTTPListen: ":80", HTTPSListen: ":443", CaddyDataDir: "/fixture",
			Certificate: model.CertificateConfig{Issuer: "letsencrypt", Subjects: subjects},
		}}).Render([]model.RouteConfig{{ID: "app", Host: "app.example.com", Enabled: true, HTTPS: true, Upstreams: []model.UpstreamTarget{{URL: "http://127.0.0.1:8080"}}}})
		if err != nil {
			t.Fatal(err)
		}
		policy, err := certificates.ParsePolicy(data, "/fixture")
		if err != nil {
			t.Fatal(err)
		}
		snapshot := certificates.Snapshot{Certificates: []certificates.Status{{Subjects: []string{"app.example.com"}}, {Subjects: []string{"*.example.com"}}}}
		certificates.Classify(&snapshot, policy)
		want := "covered"
		if explicitHost {
			want = "managed"
		}
		if snapshot.Certificates[0].Usage != want || snapshot.Certificates[1].CanArchive {
			t.Fatalf("explicit=%v inventory=%+v", explicitHost, snapshot)
		}
	}
}

func startTestCaddy(t *testing.T, binary string, data []byte, adminAddress string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "caddy.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, binary, "run", "--config", path)
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	client := &http.Client{Timeout: 100 * time.Millisecond}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		response, err := client.Get("http://" + adminAddress + "/config/")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("Caddy did not become ready")
		}
	}
}

func TestRenderProtectedRouteSkipsAutoHTTPSAndAddsFallback(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Auth: model.AuthConfig{AdminToken: "secret"},
		Gateway: model.GatewayConfig{
			HTTPListen:         ":80",
			HTTPSListen:        ":443",
			CaddyAdminEndpoint: "http://127.0.0.1:2019",
			CaddyDataDir:       "/data/caddy",
		},
	})

	data, err := renderer.Render([]model.RouteConfig{{
		ID:        "protected",
		Host:      "app.localhost",
		Exposure:  "protected",
		Enabled:   true,
		Public:    true,
		Protected: true,
		HTTPS:     false,
		Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	server := renderedServer(t, config, ":80")
	automaticHTTPS := server["automatic_https"].(map[string]any)
	if automaticHTTPS["disable"] != true {
		t.Fatalf("automatic_https = %#v", automaticHTTPS)
	}
	routes := server["routes"].([]any)
	if len(routes) != 3 {
		t.Fatalf("routes length = %d, want 3", len(routes))
	}
	fallback := renderedHandler(t, routes[2], "static_response")
	if fallback["handler"] != "static_response" || fallback["status_code"].(float64) != 401 {
		t.Fatalf("fallback handler = %#v", fallback)
	}
}

func TestRenderUsesListenerPortAndProtocol(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
	}})

	data, err := renderer.Render([]model.RouteConfig{{
		ID: "custom-listener", Host: "app.example.com", ListenerPort: 8443, ListenerProtocol: "https",
		Enabled: true, HTTPS: true, Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	server := renderedServer(t, config, ":8443")
	listens := server["listen"].([]any)
	found := false
	for _, listen := range listens {
		found = found || listen == ":8443"
	}
	if !found {
		t.Fatalf("listen = %#v, want :8443", listens)
	}
	match := server["routes"].([]any)[0].(map[string]any)["match"].([]any)[0].(map[string]any)
	if match["expression"] != "{http.request.local.port} == 8443" || match["protocol"] != "https" {
		t.Fatalf("listener match = %#v", match)
	}
	if _, exists := match["local_port"]; exists {
		t.Fatalf("listener match contains unsupported local_port matcher: %#v", match)
	}
}

func TestRenderHTTPListenerDoesNotDisableHTTPSForSameHost(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
	}})
	routes := []model.RouteConfig{
		{ID: "http", Host: "app.example.com", ListenerPort: 80, ListenerProtocol: "http", Enabled: true, Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}},
		{ID: "https", Host: "app.example.com", ListenerPort: 443, ListenerProtocol: "https", Enabled: true, HTTPS: true, Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}},
	}
	data, err := renderer.Render(routes)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	server := renderedServer(t, config, ":443")
	if automaticHTTPS := server["automatic_https"].(map[string]any); automaticHTTPS["skip"] != nil || automaticHTTPS["disable"] == true {
		t.Fatalf("automatic_https = %#v, want no skip for host with HTTPS listener", automaticHTTPS)
	}
}

func TestRenderSkipsCertificatesCoveredByConfiguredWildcard(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
		Certificate: model.CertificateConfig{Issuer: "letsencrypt", Subjects: []string{"*.example.com"}},
	}})
	routes := []model.RouteConfig{
		{ID: "covered", Host: "a.example.com", Enabled: true, HTTPS: true, Upstreams: []model.UpstreamTarget{{URL: "http://a:8080"}}},
		{ID: "nested", Host: "a.b.example.com", Enabled: true, HTTPS: true, Upstreams: []model.UpstreamTarget{{URL: "http://nested:8080"}}},
		{ID: "other", Host: "other.net", Enabled: true, HTTPS: true, Upstreams: []model.UpstreamTarget{{URL: "http://other:8080"}}},
	}
	data, err := renderer.Render(routes)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	server := renderedServer(t, config, ":443")
	automaticHTTPS := server["automatic_https"].(map[string]any)
	skipped := automaticHTTPS["skip_certificates"].([]any)
	if len(skipped) != 1 || skipped[0] != "a.example.com" {
		t.Fatalf("automatic_https.skip_certificates = %#v, want only a.example.com", skipped)
	}
}

func TestRenderSecurityEntriesUseListenerMatch(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Gateway:  model.GatewayConfig{HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy"},
		Security: model.SecurityConfig{Enabled: true, DeniedMethods: []string{"TRACE"}, DeniedPathPrefixes: []string{"/.git"}},
	})
	data, err := renderer.Render([]model.RouteConfig{{
		ID: "secure-listener", Host: "app.example.com", ListenerPort: 8443, ListenerProtocol: "https",
		Enabled: true, HTTPS: true, Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	routes := renderedServer(t, config, ":8443")["routes"].([]any)
	if len(routes) < 2 {
		t.Fatalf("routes length = %d, want security and proxy entries", len(routes))
	}
	for _, entry := range routes {
		match := entry.(map[string]any)["match"].([]any)[0].(map[string]any)
		if match["expression"] != "{http.request.local.port} == 8443" || match["protocol"] != "https" {
			t.Fatalf("listener match = %#v", match)
		}
		if _, exists := match["local_port"]; exists {
			t.Fatalf("listener match contains unsupported local_port matcher: %#v", match)
		}
	}
}

func TestRenderSetsConfiguredUpstreamHostHeader(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
	}})
	data, err := renderer.Render([]model.RouteConfig{{
		ID: "external", Host: "app.example.com", Enabled: true, Headers: map[string]string{"Host": "ex.example.com"},
		Upstreams: []model.UpstreamTarget{{URL: "https://ex.example.com:443"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	route := renderedServer(t, config, ":80")["routes"].([]any)[0].(map[string]any)
	handler := renderedHandler(t, route, "reverse_proxy")
	requestHeaders := handler["headers"].(map[string]any)["request"].(map[string]any)
	set := requestHeaders["set"].(map[string]any)
	host := set["Host"].([]any)
	if len(host) != 1 || host[0] != "ex.example.com" {
		t.Fatalf("Host header = %#v", host)
	}
}

func TestRenderManagementHostPreservesAPIAuthentication(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Control: model.ControlConfig{Listen: ":8080", ManagementHost: "admin.example.com"},
		Auth:    model.AuthConfig{Required: true, AdminToken: "secret"},
		Gateway: model.GatewayConfig{
			HTTPListen:         ":80",
			HTTPSListen:        ":443",
			CaddyAdminEndpoint: "http://127.0.0.1:2019",
			CaddyDataDir:       "/data/caddy",
		},
	})

	data, err := renderer.Render(nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	server := renderedServer(t, config, ":443")
	routes := server["routes"].([]any)
	if len(routes) != 1 {
		t.Fatalf("management route entries = %d, want one API-authenticated proxy", len(routes))
	}
	proxy := renderedHandler(t, routes[0], "reverse_proxy")
	if proxy["headers"] != nil {
		t.Fatalf("management proxy rewrites authentication headers: %#v", proxy)
	}
}

func TestRenderRejectsUnauthenticatedManagementHost(t *testing.T) {
	for _, authentication := range []model.AuthConfig{{}, {AdminToken: "secret"}, {Required: true}} {
		_, err := NewRenderer(model.AppConfig{Control: model.ControlConfig{ManagementHost: "admin.example.com"}, Auth: authentication, Gateway: model.GatewayConfig{HTTPSListen: ":443"}}).Render(nil)
		if err == nil {
			t.Fatal("management host accepted without required API authentication and tokens")
		}
	}
}

func TestRenderCertificatePolicyLetsEncryptStaging(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Gateway: model.GatewayConfig{
			HTTPListen:         ":80",
			HTTPSListen:        ":443",
			CaddyAdminEndpoint: "http://127.0.0.1:2019",
			CaddyDataDir:       "/data/caddy",
			Certificate:        model.CertificateConfig{Issuer: "letsencrypt", Email: "ops@example.com", Staging: true},
		},
	})

	data, err := renderer.Render(nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	apps := config["apps"].(map[string]any)
	tls := apps["tls"].(map[string]any)
	automation := tls["automation"].(map[string]any)
	policy := automation["policies"].([]any)[0].(map[string]any)
	issuer := policy["issuers"].([]any)[0].(map[string]any)
	if issuer["module"] != "acme" || issuer["email"] != "ops@example.com" || issuer["ca"] != "https://acme-staging-v02.api.letsencrypt.org/directory" {
		t.Fatalf("issuer = %#v", issuer)
	}
}

func TestRenderAzureDNSWildcardCertificate(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
		Certificate: model.CertificateConfig{
			Issuer: "letsencrypt", Subjects: []string{"*.example.com", "example.com"}, RenewalWindowRatio: 0.5,
			DNSChallenge: model.DNSChallengeConfig{Provider: "azure", Azure: model.AzureDNSChallengeConfig{
				SubscriptionID: "subscription", ResourceGroup: "dns-rg", Authentication: "managedidentity",
			}},
		},
	}})
	data, err := renderer.Render(nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	tls := config["apps"].(map[string]any)["tls"].(map[string]any)
	automate := tls["certificates"].(map[string]any)["automate"].([]any)
	if len(automate) != 2 || automate[0] != "*.example.com" || automate[1] != "example.com" {
		t.Fatalf("certificates.automate = %#v", automate)
	}
	policies := tls["automation"].(map[string]any)["policies"].([]any)
	if len(policies) != 2 {
		t.Fatalf("automation policies = %#v, want DNS policy and fallback", policies)
	}
	dnsPolicy := policies[0].(map[string]any)
	if dnsPolicy["renewal_window_ratio"] != 0.5 || policies[1].(map[string]any)["renewal_window_ratio"] != 0.5 {
		t.Fatalf("renewal window policies = %#v", policies)
	}
	issuer := dnsPolicy["issuers"].([]any)[0].(map[string]any)
	provider := issuer["challenges"].(map[string]any)["dns"].(map[string]any)["provider"].(map[string]any)
	if provider["name"] != "azure" || provider["subscription_id"] != "subscription" || provider["resource_group_name"] != "dns-rg" {
		t.Fatalf("Azure DNS provider = %#v", provider)
	}
	if _, ok := provider["client_secret"]; ok {
		t.Fatalf("managed identity provider contains client secret: %#v", provider)
	}
}

func TestRenderAzureDNSAppRegistrationCredentials(t *testing.T) {
	provider := azureDNSProvider(model.AzureDNSChallengeConfig{
		SubscriptionID: "subscription", ResourceGroup: "dns-rg", Authentication: "appregistration",
		TenantID: "tenant", ClientID: "client", ClientSecret: "secret",
	})
	if provider["tenant_id"] != "tenant" || provider["client_id"] != "client" || provider["client_secret"] != "secret" {
		t.Fatalf("Azure DNS provider = %#v", provider)
	}
}

func TestRenderProtectedRouteWithCustomHeaderPolicy(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Auth: model.AuthConfig{ProtectedRoutes: model.ProtectedRouteConfig{AdditionalHeaderName: "X-Gateway-Token", AdditionalHeaderValue: "edge-secret"}},
		Gateway: model.GatewayConfig{
			HTTPListen:         ":80",
			HTTPSListen:        ":443",
			CaddyAdminEndpoint: "http://127.0.0.1:2019",
			CaddyDataDir:       "/data/caddy",
		},
	})

	data, err := renderer.Render([]model.RouteConfig{{
		ID:        "protected",
		Host:      "app.localhost",
		Exposure:  "protected",
		Enabled:   true,
		Public:    true,
		Protected: true,
		HTTPS:     true,
		Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	server := renderedServer(t, config, ":443")
	routes := server["routes"].([]any)
	if len(routes) != 2 {
		t.Fatalf("routes length = %d, want custom header route plus fallback", len(routes))
	}
	match := routes[0].(map[string]any)["match"].([]any)[0].(map[string]any)
	header := match["header"].(map[string]any)
	values := header["X-Gateway-Token"].([]any)
	if len(values) != 1 || values[0] != "edge-secret" {
		t.Fatalf("custom header match = %#v", header)
	}
}

func TestRenderRejectsUnsupportedUpstreamScheme(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Gateway: model.GatewayConfig{
			HTTPListen:         ":80",
			HTTPSListen:        ":443",
			CaddyAdminEndpoint: "http://127.0.0.1:2019",
			CaddyDataDir:       "/data/caddy",
		},
	})

	_, err := renderer.Render([]model.RouteConfig{{
		ID:        "bad-upstream",
		Host:      "app.localhost",
		Enabled:   true,
		Public:    true,
		Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "tcp://svc:8080"}},
	}})
	if err == nil {
		t.Fatal("Render() error = nil, want unsupported scheme error")
	}
}

func TestRenderInternalRouteRestrictsRemoteIP(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Gateway: model.GatewayConfig{
			HTTPListen:           ":80",
			HTTPSListen:          ":443",
			CaddyAdminEndpoint:   "http://127.0.0.1:2019",
			CaddyDataDir:         "/data/caddy",
			InternalSourceRanges: []string{"10.0.0.0/8"},
		},
	})

	data, err := renderer.Render([]model.RouteConfig{{
		ID: "internal", Host: "internal.example.com", Exposure: "internal", Enabled: true,
		Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "http://svc:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	routes := renderedServer(t, config, ":80")["routes"].([]any)
	match := routes[0].(map[string]any)["match"].([]any)[0].(map[string]any)
	remoteIP := match["remote_ip"].(map[string]any)
	ranges := remoteIP["ranges"].([]any)
	if len(ranges) != 1 || ranges[0] != "10.0.0.0/8" {
		t.Fatalf("remote_ip ranges = %#v", ranges)
	}
}

func TestRenderOrdersLongerPathPrefixesFirst(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
	}})
	routes := []model.RouteConfig{
		{ID: "root", Host: "app.example.com", Enabled: true, Public: true, Upstreams: []model.UpstreamTarget{{Name: "root", URL: "http://root:8080"}}},
		{ID: "api", Host: "app.example.com", PathPrefix: "/api", Enabled: true, Public: true, Upstreams: []model.UpstreamTarget{{Name: "api", URL: "http://api:8080"}}},
		{ID: "admin", Host: "app.example.com", PathPrefix: "/api/admin/", Enabled: true, Public: true, Upstreams: []model.UpstreamTarget{{Name: "admin", URL: "http://admin:8080"}}},
	}

	data, err := renderer.Render(routes)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	rendered := renderedServer(t, config, ":80")["routes"].([]any)
	firstMatch := rendered[0].(map[string]any)["match"].([]any)[0].(map[string]any)
	paths := firstMatch["path"].([]any)
	if len(paths) != 2 || paths[0] != "/api/admin" || paths[1] != "/api/admin/*" {
		t.Fatalf("first route paths = %#v", paths)
	}
	secondMatch := rendered[1].(map[string]any)["match"].([]any)[0].(map[string]any)
	if secondMatch["path"].([]any)[0] != "/api" {
		t.Fatalf("second route match = %#v", secondMatch)
	}
}

func TestRenderOrdersExactHostBeforeWildcard(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
	}})
	data, err := renderer.Render([]model.RouteConfig{
		{ID: "wildcard", Host: "*.example.com", Enabled: true, Upstreams: []model.UpstreamTarget{{URL: "http://default:8080"}}},
		{ID: "exact", Host: "api.example.com", Enabled: true, Upstreams: []model.UpstreamTarget{{URL: "http://api:8080"}}},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	routes := renderedServer(t, config, ":80")["routes"].([]any)
	match := routes[0].(map[string]any)["match"].([]any)[0].(map[string]any)
	if match["host"].([]any)[0] != "api.example.com" {
		t.Fatalf("first route match = %#v, want exact host", match)
	}
}

func TestRenderRejectsMixedUpstreamSchemes(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
	}})
	_, err := renderer.Render([]model.RouteConfig{{
		ID: "mixed", Host: "app.example.com", Enabled: true, Public: true,
		Upstreams: []model.UpstreamTarget{{Name: "http", URL: "http://one:8080"}, {Name: "https", URL: "https://two:8443"}},
	}})
	if err == nil {
		t.Fatal("Render() error = nil, want mixed upstream scheme error")
	}
}

func TestRenderEnablesAccessLogsWithRouteMetadata(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
	}})
	data, err := renderer.Render([]model.RouteConfig{{
		ID: "rule-api", Name: "API", Host: "api.example.com", ListenerID: "listener-public", ListenerName: "Public HTTPS",
		ListenerPort: 443, ListenerProtocol: "https", BackendPoolID: "pool-api", BackendPoolName: "API nodes",
		Enabled: true, Upstreams: []model.UpstreamTarget{{URL: "http://api:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	server := renderedServer(t, config, ":443")
	if _, enabled := server["logs"].(map[string]any); !enabled {
		t.Fatalf("server logs = %#v, want enabled access logs", server["logs"])
	}
	handlers := server["routes"].([]any)[0].(map[string]any)["handle"].([]any)
	fields := make(map[string]any)
	for _, value := range handlers {
		handler := value.(map[string]any)
		if handler["handler"] == "log_append" {
			fields[handler["key"].(string)] = handler["value"]
		}
	}
	want := map[string]any{
		"route_id": "rule-api", "route_name": "API", "route_host": "api.example.com",
		"listener_id": "listener-public", "listener_name": "Public HTTPS", "listener_protocol": "https",
		"listener_port": "{http.request.local.port}", "backend_pool_id": "pool-api", "backend_pool_name": "API nodes",
		"upstream_host": "{http.reverse_proxy.upstream.host}", "upstream_duration_ms": "{http.reverse_proxy.upstream.duration_ms}",
		"upstream_latency_ms": "{http.reverse_proxy.upstream.latency_ms}",
	}
	for key, expected := range want {
		if fields[key] != expected {
			t.Errorf("access log field %s = %#v, want %#v", key, fields[key], expected)
		}
	}
}

func TestRenderProtectedRouteStripsGatewayCredentials(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Auth:    model.AuthConfig{AdminToken: "secret", ProtectedRoutes: model.ProtectedRouteConfig{AllowBearerToken: true, AllowAdminTokenHeader: true}},
		Gateway: model.GatewayConfig{HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy"},
	})
	data, err := renderer.Render([]model.RouteConfig{{
		ID: "protected", Host: "app.example.com", Exposure: "protected", Enabled: true, Public: true, Protected: true,
		Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	routes := renderedServer(t, config, ":80")["routes"].([]any)
	handler := renderedHandler(t, routes[0], "reverse_proxy")
	requestHeaders := handler["headers"].(map[string]any)["request"].(map[string]any)
	deleted := requestHeaders["delete"].([]any)
	if len(deleted) != 2 || deleted[0] != "Authorization" || deleted[1] != "X-Admin-Token" {
		t.Fatalf("deleted request headers = %#v", deleted)
	}
}

func renderedServer(t *testing.T, config map[string]any, address string) map[string]any {
	t.Helper()
	servers := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)
	for _, value := range servers {
		server := value.(map[string]any)
		for _, listen := range server["listen"].([]any) {
			if listen == address {
				return server
			}
		}
	}
	t.Fatalf("no server listening on %s", address)
	return nil
}

func TestRenderRejectsConflictingListenerProtocols(t *testing.T) {
	_, err := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{HTTPListen: ":8443", HTTPSListen: "127.0.0.1:8443"}}).Render(nil)
	if err == nil {
		t.Fatal("expected conflicting listener protocols to fail")
	}
}

func renderedHandler(t *testing.T, route any, handlerType string) map[string]any {
	t.Helper()
	for _, value := range route.(map[string]any)["handle"].([]any) {
		handler := value.(map[string]any)
		if handler["handler"] == handlerType {
			return handler
		}
	}
	t.Fatalf("handler %q not found in %#v", handlerType, route)
	return nil
}

func TestRenderSecurityBaselineBeforeReverseProxy(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Security: model.SecurityConfig{
			Enabled:             true,
			MaxRequestBodyBytes: 1024,
			DeniedMethods:       []string{"TRACE", "CONNECT"},
			DeniedPathPrefixes:  []string{"/.git", "/api/private"},
			AllowedCIDRs:        []string{"10.0.0.0/8"},
			BlockedCIDRs:        []string{"10.0.0.5"},
		},
		Gateway: model.GatewayConfig{HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy"},
	})
	data, err := renderer.Render([]model.RouteConfig{{
		ID: "api", Host: "app.example.com", PathPrefix: "/api", Enabled: true,
		Security:  model.RouteSecurityConfig{AllowedCIDRs: []string{"10.1.0.0/16"}, AdditionalDeniedMethods: []string{"M-SEARCH"}},
		Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	routes := renderedServer(t, config, ":80")["routes"].([]any)
	if len(routes) != 6 {
		t.Fatalf("routes length = %d, want 5 security entries plus proxy", len(routes))
	}
	blockedMatch := routes[0].(map[string]any)["match"].([]any)[0].(map[string]any)
	blockedRanges := blockedMatch["remote_ip"].(map[string]any)["ranges"].([]any)
	if len(blockedRanges) != 1 || blockedRanges[0] != "10.0.0.5" {
		t.Fatalf("blocked ranges = %#v", blockedRanges)
	}
	routeAllowMatch := routes[2].(map[string]any)["match"].([]any)[0].(map[string]any)
	notMatchers := routeAllowMatch["not"].([]any)
	allowedRanges := notMatchers[0].(map[string]any)["remote_ip"].(map[string]any)["ranges"].([]any)
	if len(allowedRanges) != 1 || allowedRanges[0] != "10.1.0.0/16" {
		t.Fatalf("route allowed ranges = %#v", allowedRanges)
	}
	methodMatch := routes[3].(map[string]any)["match"].([]any)[0].(map[string]any)
	methods := methodMatch["method"].([]any)
	if len(methods) != 3 || methods[2] != "M-SEARCH" {
		t.Fatalf("denied methods = %#v", methods)
	}
	pathMatch := routes[4].(map[string]any)["match"].([]any)[0].(map[string]any)
	paths := pathMatch["path"].([]any)
	if len(paths) != 2 || paths[0] != "/api/private" || paths[1] != "/api/private/*" {
		t.Fatalf("denied paths = %#v", paths)
	}
	proxyHandlers := routes[5].(map[string]any)["handle"].([]any)
	requestBodyIndex := -1
	proxyIndex := -1
	for index, value := range proxyHandlers {
		handler := value.(map[string]any)
		if handler["handler"] == "request_body" {
			requestBodyIndex = index
			if handler["max_size"].(float64) != 1024 {
				t.Fatalf("request body handler = %#v", handler)
			}
		}
		if handler["handler"] == "reverse_proxy" {
			proxyIndex = index
		}
	}
	if requestBodyIndex < 0 || proxyIndex < 0 || requestBodyIndex >= proxyIndex {
		t.Fatalf("proxy handlers = %#v", proxyHandlers)
	}
}

func TestRenderRouteCanDisableSecurityBaseline(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Security: model.SecurityConfig{Enabled: true, MaxRequestBodyBytes: 1024, DeniedMethods: []string{"TRACE"}},
		Gateway:  model.GatewayConfig{HTTPListen: ":80", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy"},
	})
	data, err := renderer.Render([]model.RouteConfig{{
		ID: "upload", Host: "upload.example.com", Enabled: true, Security: model.RouteSecurityConfig{Disabled: true},
		Upstreams: []model.UpstreamTarget{{URL: "http://upload:8080"}},
	}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	routes := renderedServer(t, config, ":80")["routes"].([]any)
	if len(routes) != 1 {
		t.Fatalf("routes = %#v, want one proxy route", routes)
	}
	for _, value := range routes[0].(map[string]any)["handle"].([]any) {
		if value.(map[string]any)["handler"] == "request_body" {
			t.Fatalf("routes = %#v, want no request body limit", routes)
		}
	}
}
