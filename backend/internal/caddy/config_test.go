package caddy

import (
	"encoding/json"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

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
	server := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)
	automaticHTTPS := server["automatic_https"].(map[string]any)
	skip := automaticHTTPS["skip"].([]any)
	if len(skip) != 1 || skip[0] != "app.localhost" {
		t.Fatalf("automatic_https.skip = %#v", skip)
	}
	routes := server["routes"].([]any)
	if len(routes) != 3 {
		t.Fatalf("routes length = %d, want 3", len(routes))
	}
	fallback := routes[2].(map[string]any)["handle"].([]any)[0].(map[string]any)
	if fallback["handler"] != "static_response" || fallback["status_code"].(float64) != 401 {
		t.Fatalf("fallback handler = %#v", fallback)
	}
}

func TestRenderManagementHostIsProtected(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{
		Control: model.ControlConfig{Listen: ":8080", ManagementHost: "admin.example.com"},
		Auth:    model.AuthConfig{AdminToken: "secret"},
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
	server := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)
	routes := server["routes"].([]any)
	if len(routes) != 3 {
		t.Fatalf("management route entries = %d, want 3 protected entries", len(routes))
	}
	fallback := routes[2].(map[string]any)["handle"].([]any)[0].(map[string]any)
	if fallback["handler"] != "static_response" || fallback["status_code"].(float64) != 401 {
		t.Fatalf("management fallback handler = %#v", fallback)
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
	server := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)
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
