package caddy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
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

func (r *Renderer) UpdateConfig(cfg model.AppConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg
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
	sort.SliceStable(routeList, func(left int, right int) bool {
		leftHost := strings.ToLower(routeList[left].Host)
		rightHost := strings.ToLower(routeList[right].Host)
		leftWildcard := strings.HasPrefix(leftHost, "*.")
		rightWildcard := strings.HasPrefix(rightHost, "*.")
		if leftWildcard != rightWildcard {
			return !leftWildcard
		}
		if leftHost != rightHost {
			return leftHost < rightHost
		}
		return len(strings.TrimRight(routeList[left].PathPrefix, "/")) > len(strings.TrimRight(routeList[right].PathPrefix, "/"))
	})

	listen := make([]string, 0, 2+len(routeList))
	listenSet := make(map[string]struct{}, 2+len(routeList))
	appendListen := func(address string) {
		if address == "" {
			return
		}
		if _, exists := listenSet[address]; exists {
			return
		}
		listenSet[address] = struct{}{}
		listen = append(listen, address)
	}
	if cfg.Gateway.HTTPListen != "" {
		appendListen(cfg.Gateway.HTTPListen)
	}
	if cfg.Gateway.HTTPSListen != "" {
		appendListen(cfg.Gateway.HTTPSListen)
	}
	for _, route := range routeList {
		if route.ListenerPort > 0 {
			appendListen(listenerAddress(cfg.Gateway, route.ListenerProtocol, route.ListenerPort))
		}
	}
	if len(listen) == 0 {
		return nil, fmt.Errorf("at least one gateway listener is required")
	}

	caddyRoutes := make([]any, 0, len(routeList))
	skipAutoHTTPS := make([]string, 0)
	httpsHosts := make(map[string]struct{}, len(routeList))
	for _, route := range routeList {
		if route.Enabled && route.HTTPS {
			httpsHosts[strings.ToLower(route.Host)] = struct{}{}
		}
	}
	skippedHosts := make(map[string]struct{}, len(routeList))
	for _, route := range routeList {
		if !route.Enabled {
			continue
		}
		if !route.HTTPS {
			host := strings.ToLower(route.Host)
			_, hasHTTPS := httpsHosts[host]
			_, alreadySkipped := skippedHosts[host]
			if !hasHTTPS && !alreadySkipped {
				skipAutoHTTPS = append(skipAutoHTTPS, route.Host)
				skippedHosts[host] = struct{}{}
			}
		}
		entries, err := renderRoute(route, cfg.Auth, cfg.Security, cfg.Gateway.InternalSourceRanges)
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

func renderRoute(route model.RouteConfig, auth model.AuthConfig, security model.SecurityConfig, internalSourceRanges []string) ([]map[string]any, error) {
	match := map[string]any{"host": []string{route.Host}}
	if route.ListenerPort > 0 {
		match["local_port"] = []int{route.ListenerPort}
	}
	if route.ListenerProtocol != "" {
		match["protocol"] = route.ListenerProtocol
	}
	if route.PathPrefix != "" {
		match["path"] = routePathMatchers(route.PathPrefix)
	}
	if route.Exposure == "internal" {
		if len(internalSourceRanges) == 0 {
			return nil, fmt.Errorf("route %s: internal route requires at least one source range", route.ID)
		}
		match["remote_ip"] = map[string]any{"ranges": internalSourceRanges}
	}

	upstreams := make([]any, 0, len(route.Upstreams))
	useTLS := false
	transportSet := false
	for _, upstream := range route.Upstreams {
		dial, tls, err := upstreamDial(upstream.URL)
		if err != nil {
			return nil, fmt.Errorf("route %s: %w", route.ID, err)
		}
		if transportSet && useTLS != tls {
			return nil, fmt.Errorf("route %s: all upstreams must use the same http or https scheme", route.ID)
		}
		useTLS = tls
		transportSet = true
		upstreams = append(upstreams, map[string]any{"dial": dial})
	}
	securityEntries, maxRequestBodyBytes, err := renderSecurityEntries(route, security)
	if err != nil {
		return nil, err
	}

	if !route.Protected {
		handler := reverseProxyHandler(upstreams, useTLS, nil, route.Headers)
		entry := map[string]any{"match": []any{match}, "handle": proxyHandlers(maxRequestBodyBytes, handler), "terminal": true}
		return append(securityEntries, entry), nil
	}

	fallbackStatus := 401
	fallbackBody := "protected route requires gateway token\n"
	policies := protectedHeaderPolicies(auth)
	if len(policies) == 0 {
		fallbackStatus = 503
		fallbackBody = "protected route policy is not configured\n"
	}
	fallback := map[string]any{
		"match":    []any{match},
		"handle":   []any{map[string]any{"handler": "static_response", "status_code": fallbackStatus, "body": fallbackBody}},
		"terminal": true,
	}
	if len(policies) == 0 {
		return append(securityEntries, fallback), nil
	}

	stripHeaders := uniqueHeaderNames(policies)
	entries := make([]map[string]any, 0, len(securityEntries)+len(policies)+1)
	entries = append(entries, securityEntries...)
	for _, header := range policies {
		headerMatch := cloneMatch(match)
		headerMatch["header"] = map[string]any{header.name: []string{header.value}}
		handler := reverseProxyHandler(upstreams, useTLS, stripHeaders, route.Headers)
		entries = append(entries, map[string]any{"match": []any{headerMatch}, "handle": proxyHandlers(maxRequestBodyBytes, handler), "terminal": true})
	}
	entries = append(entries, fallback)
	return entries, nil
}

func renderSecurityEntries(route model.RouteConfig, security model.SecurityConfig) ([]map[string]any, int64, error) {
	if !security.Enabled || route.Security.Disabled {
		return nil, 0, nil
	}
	maxRequestBodyBytes := security.MaxRequestBodyBytes
	if route.Security.MaxRequestBodyBytes != 0 {
		maxRequestBodyBytes = route.Security.MaxRequestBodyBytes
	}
	if maxRequestBodyBytes < 0 {
		return nil, 0, fmt.Errorf("route %s: security maxRequestBodyBytes must not be negative", route.ID)
	}

	baseMatch := map[string]any{"host": []string{route.Host}}
	if route.ListenerPort > 0 {
		baseMatch["local_port"] = []int{route.ListenerPort}
	}
	if route.ListenerProtocol != "" {
		baseMatch["protocol"] = route.ListenerProtocol
	}
	if route.PathPrefix != "" {
		baseMatch["path"] = routePathMatchers(route.PathPrefix)
	}
	entries := make([]map[string]any, 0, 5)
	blockedCIDRs := appendUnique(security.BlockedCIDRs, route.Security.BlockedCIDRs...)
	if len(blockedCIDRs) > 0 {
		match := cloneMatch(baseMatch)
		match["remote_ip"] = map[string]any{"ranges": blockedCIDRs}
		entries = append(entries, securityRejection(match, 403))
	}
	for _, allowedCIDRs := range [][]string{security.AllowedCIDRs, route.Security.AllowedCIDRs} {
		if len(allowedCIDRs) == 0 {
			continue
		}
		match := cloneMatch(baseMatch)
		match["not"] = []any{map[string]any{"remote_ip": map[string]any{"ranges": allowedCIDRs}}}
		entries = append(entries, securityRejection(match, 403))
	}
	deniedMethods := appendUnique(security.DeniedMethods, route.Security.AdditionalDeniedMethods...)
	if len(deniedMethods) > 0 {
		match := cloneMatch(baseMatch)
		match["method"] = deniedMethods
		entries = append(entries, securityRejection(match, 405))
	}
	deniedPaths := appendUnique(security.DeniedPathPrefixes, route.Security.AdditionalDeniedPathPrefixes...)
	if paths := restrictedPathMatchers(route.PathPrefix, deniedPaths); len(paths) > 0 {
		match := cloneMatch(baseMatch)
		match["path"] = paths
		entries = append(entries, securityRejection(match, 403))
	}
	return entries, maxRequestBodyBytes, nil
}

func securityRejection(match map[string]any, status int) map[string]any {
	return map[string]any{
		"match": []any{match},
		"handle": []any{map[string]any{
			"handler":     "static_response",
			"status_code": status,
			"body":        "request blocked by gateway security policy\n",
		}},
		"terminal": true,
	}
}

func restrictedPathMatchers(routePrefix string, deniedPrefixes []string) []string {
	output := make([]string, 0, len(deniedPrefixes)*2)
	for _, deniedPrefix := range deniedPrefixes {
		restrictedPrefix, ok := intersectPathPrefixes(routePrefix, deniedPrefix)
		if !ok {
			continue
		}
		output = appendUnique(output, routePathMatchers(restrictedPrefix)...)
	}
	return output
}

func intersectPathPrefixes(left string, right string) (string, bool) {
	left = strings.TrimRight(strings.TrimSpace(left), "/")
	right = strings.TrimRight(strings.TrimSpace(right), "/")
	if left == "" {
		return right, true
	}
	if right == "" {
		return left, true
	}
	if left == right || strings.HasPrefix(left, right+"/") {
		return left, true
	}
	if strings.HasPrefix(right, left+"/") {
		return right, true
	}
	return "", false
}

func appendUnique(base []string, values ...string) []string {
	output := append([]string{}, base...)
	seen := make(map[string]struct{}, len(output)+len(values))
	for _, value := range output {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		output = append(output, value)
	}
	return output
}

func routePathMatchers(pathPrefix string) []string {
	prefix := strings.TrimRight(strings.TrimSpace(pathPrefix), "/")
	if prefix == "" {
		return []string{"/*"}
	}
	return []string{prefix, prefix + "/*"}
}

func reverseProxyHandler(upstreams []any, useTLS bool, stripHeaders []string, setHeaders map[string]string) map[string]any {
	handler := map[string]any{"handler": "reverse_proxy", "upstreams": upstreams}
	if useTLS {
		handler["transport"] = map[string]any{"protocol": "http", "tls": map[string]any{}}
	}
	requestHeaders := make(map[string]any, 2)
	if len(stripHeaders) > 0 {
		requestHeaders["delete"] = stripHeaders
	}
	set := make(map[string][]string, len(setHeaders))
	for name, value := range setHeaders {
		name = strings.TrimSpace(name)
		if name == "" || headerNameIn(name, stripHeaders) {
			continue
		}
		set[name] = []string{value}
	}
	if len(set) > 0 {
		requestHeaders["set"] = set
	}
	if len(requestHeaders) > 0 {
		handler["headers"] = map[string]any{"request": requestHeaders}
	}
	return handler
}

func headerNameIn(name string, names []string) bool {
	for _, candidate := range names {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

func proxyHandlers(maxRequestBodyBytes int64, proxy map[string]any) []any {
	handlers := make([]any, 0, 2)
	if maxRequestBodyBytes > 0 {
		handlers = append(handlers, map[string]any{"handler": "request_body", "max_size": maxRequestBodyBytes})
	}
	return append(handlers, proxy)
}

func uniqueHeaderNames(policies []headerPolicy) []string {
	seen := make(map[string]struct{}, len(policies))
	output := make([]string, 0, len(policies))
	for _, policy := range policies {
		key := strings.ToLower(policy.name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		output = append(output, policy.name)
	}
	return output
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
	policies := make([]any, 0, 2)
	if cfg.DNSChallenge.Provider != "" && len(cfg.Subjects) > 0 {
		policies = append(policies, map[string]any{
			"subjects": cfg.Subjects,
			"issuers":  []any{certificateIssuer(cfg, true)},
		})
		policies = append(policies, map[string]any{"issuers": []any{certificateIssuer(cfg, false)}})
	} else {
		policies = append(policies, map[string]any{"issuers": []any{certificateIssuer(cfg, false)}})
	}
	output := map[string]any{"automation": map[string]any{"policies": policies}}
	if len(cfg.Subjects) > 0 {
		output["certificates"] = map[string]any{"automate": cfg.Subjects}
	}
	return output
}

func certificateIssuer(cfg model.CertificateConfig, dnsChallenge bool) map[string]any {
	issuer := strings.ToLower(strings.TrimSpace(cfg.Issuer))
	var output map[string]any
	switch issuer {
	case "zerossl":
		output = map[string]any{"module": "zerossl"}
		if cfg.Email != "" {
			output["email"] = cfg.Email
		}
	case "custom":
		output = map[string]any{"module": "acme"}
		if cfg.CADirectory != "" {
			output["ca"] = cfg.CADirectory
		}
		if cfg.Email != "" {
			output["email"] = cfg.Email
		}
	default:
		output = map[string]any{"module": "acme"}
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
	}
	if dnsChallenge {
		output["challenges"] = map[string]any{
			"dns": map[string]any{"provider": azureDNSProvider(cfg.DNSChallenge.Azure)},
		}
	}
	return output
}

func azureDNSProvider(cfg model.AzureDNSChallengeConfig) map[string]any {
	provider := map[string]any{
		"name":                "azure",
		"subscription_id":     cfg.SubscriptionID,
		"resource_group_name": cfg.ResourceGroup,
	}
	if cfg.Authentication == "appregistration" {
		provider["tenant_id"] = cfg.TenantID
		provider["client_id"] = cfg.ClientID
		provider["client_secret"] = cfg.ClientSecret
	}
	return provider
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

func listenerAddress(cfg model.GatewayConfig, protocol string, port int) string {
	base := cfg.HTTPListen
	if strings.EqualFold(protocol, "https") {
		base = cfg.HTTPSListen
	}
	host, _, err := net.SplitHostPort(base)
	if err != nil {
		return ":" + strconv.Itoa(port)
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func adminListenFromEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "127.0.0.1:2019"
	}
	return parsed.Host
}
