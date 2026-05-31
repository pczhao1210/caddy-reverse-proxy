package caddy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/aidockerfarm/gateway/internal/model"
)

type Renderer struct {
	mu  sync.RWMutex
	cfg model.AppConfig
}

func NewRenderer(cfg model.AppConfig) *Renderer {
	return &Renderer{cfg: cfg}
}

func (r *Renderer) UpdateCertificate(cert model.CertificateConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg.Gateway.Certificate = cert
}

func (r *Renderer) Render(input []model.RouteConfig) ([]byte, error) {
	r.mu.RLock()
	cfg := r.cfg
	r.mu.RUnlock()

	routeList := append([]model.RouteConfig{}, input...)
	if cfg.Control.ManagementHost != "" {
		routeList = append(routeList, model.RouteConfig{
			ID:        "management-ui",
			Host:      cfg.Control.ManagementHost,
			Exposure:  "protected",
			Enabled:   true,
			Public:    true,
			HTTPS:     true,
			Protected: true,
			Source:    "management",
			Upstreams: []model.UpstreamTarget{
				{Name: "management-ui", URL: "http://" + loopbackTarget(cfg.Control.Listen)},
			},
		})
	}

	listen := make([]string, 0, 2)
	if cfg.Gateway.HTTPListen != "" {
		listen = append(listen, cfg.Gateway.HTTPListen)
	}
	if cfg.Gateway.HTTPSListen != "" {
		listen = append(listen, cfg.Gateway.HTTPSListen)
	}
	if len(listen) == 0 {
		return nil, fmt.Errorf("at least one gateway listener is required")
	}

	caddyRoutes := make([]any, 0, len(routeList))
	skipAutoHTTPS := make([]string, 0)
	for _, route := range routeList {
		if !route.Enabled || !route.Public {
			continue
		}
		if !route.HTTPS {
			skipAutoHTTPS = append(skipAutoHTTPS, route.Host)
		}
		entries, err := renderRoute(route, cfg.Auth)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			caddyRoutes = append(caddyRoutes, entry)
		}
	}

	server := map[string]any{
		"listen": listen,
		"routes": caddyRoutes,
	}
	if len(skipAutoHTTPS) > 0 {
		server["automatic_https"] = map[string]any{"skip": skipAutoHTTPS}
	}

	config := map[string]any{
		"admin": map[string]any{
			"listen": adminListenFromEndpoint(cfg.Gateway.CaddyAdminEndpoint),
		},
		"storage": map[string]any{
			"module": "file_system",
			"root":   cfg.Gateway.CaddyDataDir,
		},
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"gateway": server,
				},
			},
		},
	}
	if tlsConfig := tlsAutomation(cfg.Gateway.Certificate); tlsConfig != nil {
		config["apps"].(map[string]any)["tls"] = tlsConfig
	}

	return json.MarshalIndent(config, "", "  ")
}

func renderRoute(route model.RouteConfig, auth model.AuthConfig) ([]map[string]any, error) {
	match := map[string]any{"host": []string{route.Host}}
	if route.PathPrefix != "" {
		match["path"] = []string{strings.TrimRight(route.PathPrefix, "/") + "*"}
	}

	upstreams := make([]any, 0, len(route.Upstreams))
	useTLS := false
	for _, upstream := range route.Upstreams {
		dial, tls, err := upstreamDial(upstream.URL)
		if err != nil {
			return nil, fmt.Errorf("route %s: %w", route.ID, err)
		}
		useTLS = useTLS || tls
		upstreams = append(upstreams, map[string]any{"dial": dial})
	}

	handler := map[string]any{"handler": "reverse_proxy", "upstreams": upstreams}
	if useTLS {
		handler["transport"] = map[string]any{"protocol": "http", "tls": map[string]any{}}
	}

	if !route.Protected {
		return []map[string]any{{"match": []any{match}, "handle": []any{handler}, "terminal": true}}, nil
	}

	fallbackStatus := 401
	fallbackBody := "protected route requires gateway token\n"
	if !protectedPolicyConfigured(auth) {
		fallbackStatus = 503
		fallbackBody = "protected route policy is not configured\n"
	}
	fallback := map[string]any{
		"match":    []any{match},
		"handle":   []any{map[string]any{"handler": "static_response", "status_code": fallbackStatus, "body": fallbackBody}},
		"terminal": true,
	}
	if !protectedPolicyConfigured(auth) {
		return []map[string]any{fallback}, nil
	}

	entries := make([]map[string]any, 0, 3)
	for _, header := range protectedHeaderPolicies(auth) {
		headerMatch := cloneMatch(match)
		headerMatch["header"] = map[string]any{header.name: []string{header.value}}
		entries = append(entries, map[string]any{"match": []any{headerMatch}, "handle": []any{handler}, "terminal": true})
	}
	entries = append(entries, fallback)
	return entries, nil
}

type headerPolicy struct {
	name  string
	value string
}

func protectedPolicyConfigured(auth model.AuthConfig) bool {
	return len(protectedHeaderPolicies(auth)) > 0
}

func protectedHeaderPolicies(auth model.AuthConfig) []headerPolicy {
	tokens := authTokens(auth)
	policies := make([]headerPolicy, 0, len(tokens)*2+1)
	allowBearer := auth.ProtectedRoutes.AllowBearerToken
	allowAdminHeader := auth.ProtectedRoutes.AllowAdminTokenHeader
	if !allowBearer && !allowAdminHeader && auth.ProtectedRoutes.AdditionalHeaderName == "" && auth.ProtectedRoutes.AdditionalHeaderValue == "" {
		allowBearer = true
		allowAdminHeader = true
	}
	for _, token := range tokens {
		if allowBearer {
			policies = append(policies, headerPolicy{name: "Authorization", value: "Bearer " + token})
		}
		if allowAdminHeader {
			policies = append(policies, headerPolicy{name: "X-Admin-Token", value: token})
		}
	}
	if auth.ProtectedRoutes.AdditionalHeaderName != "" && auth.ProtectedRoutes.AdditionalHeaderValue != "" {
		policies = append(policies, headerPolicy{name: auth.ProtectedRoutes.AdditionalHeaderName, value: auth.ProtectedRoutes.AdditionalHeaderValue})
	}
	return policies
}

func authTokens(auth model.AuthConfig) []string {
	output := make([]string, 0, 1+len(auth.AdminTokens))
	if strings.TrimSpace(auth.AdminToken) != "" {
		output = append(output, auth.AdminToken)
	}
	for _, token := range auth.AdminTokens {
		if strings.TrimSpace(token) != "" {
			output = append(output, token)
		}
	}
	return output
}

func tlsAutomation(cfg model.CertificateConfig) map[string]any {
	issuer := strings.ToLower(strings.TrimSpace(cfg.Issuer))
	if issuer == "" || issuer == "default" {
		return nil
	}
	policy := map[string]any{"issuers": []any{certificateIssuer(cfg)}}
	return map[string]any{"automation": map[string]any{"policies": []any{policy}}}
}

func certificateIssuer(cfg model.CertificateConfig) map[string]any {
	issuer := strings.ToLower(strings.TrimSpace(cfg.Issuer))
	switch issuer {
	case "zerossl":
		output := map[string]any{"module": "zerossl"}
		if cfg.Email != "" {
			output["email"] = cfg.Email
		}
		return output
	case "custom":
		output := map[string]any{"module": "acme"}
		if cfg.CADirectory != "" {
			output["ca"] = cfg.CADirectory
		}
		if cfg.Email != "" {
			output["email"] = cfg.Email
		}
		return output
	default:
		output := map[string]any{"module": "acme"}
		if cfg.Staging {
			output["ca"] = "https://acme-staging-v02.api.letsencrypt.org/directory"
		} else if cfg.CADirectory != "" {
			output["ca"] = cfg.CADirectory
		} else {
			output["ca"] = "https://acme-v02.api.letsencrypt.org/directory"
		}
		if cfg.Email != "" {
			output["email"] = cfg.Email
		}
		return output
	}
}

func cloneMatch(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func upstreamDial(raw string) (string, bool, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false, err
	}
	if parsed.Host == "" {
		return "", false, fmt.Errorf("upstream %q must include host", raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		return parsed.Host, false, nil
	case "https":
		return parsed.Host, true, nil
	default:
		return "", false, fmt.Errorf("upstream %q must use http or https", raw)
	}
}

func loopbackTarget(listen string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		if strings.HasPrefix(listen, ":") {
			return "127.0.0.1" + listen
		}
		return listen
	}
	return net.JoinHostPort("127.0.0.1", port)
}

func adminListenFromEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "127.0.0.1:2019"
	}
	return parsed.Host
}
