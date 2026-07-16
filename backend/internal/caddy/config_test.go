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

func TestRenderAzureDNSWildcardCertificate(t *testing.T) {
	renderer := NewRenderer(model.AppConfig{Gateway: model.GatewayConfig{
		HTTPListen: ":80", HTTPSListen: ":443", CaddyAdminEndpoint: "http://127.0.0.1:2019", CaddyDataDir: "/data/caddy",
		Certificate: model.CertificateConfig{
			Issuer: "letsencrypt", Subjects: []string{"*.example.com", "example.com"},
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
	routes := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)["routes"].([]any)
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
	rendered := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)["routes"].([]any)
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
	routes := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)["routes"].([]any)
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
	routes := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)["routes"].([]any)
	handler := routes[0].(map[string]any)["handle"].([]any)[0].(map[string]any)
	requestHeaders := handler["headers"].(map[string]any)["request"].(map[string]any)
	deleted := requestHeaders["delete"].([]any)
	if len(deleted) != 2 || deleted[0] != "Authorization" || deleted[1] != "X-Admin-Token" {
		t.Fatalf("deleted request headers = %#v", deleted)
	}
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
	routes := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)["routes"].([]any)
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
	if len(proxyHandlers) != 2 || proxyHandlers[0].(map[string]any)["handler"] != "request_body" || proxyHandlers[0].(map[string]any)["max_size"].(float64) != 1024 {
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
	routes := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["gateway"].(map[string]any)["routes"].([]any)
	if len(routes) != 1 || len(routes[0].(map[string]any)["handle"].([]any)) != 1 {
		t.Fatalf("routes = %#v, want unmodified proxy route", routes)
	}
}
