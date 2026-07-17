package routes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestAddNormalizesInternalRoute(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	route, err := store.Add(model.RouteConfig{
		Host:     "Internal.Localhost",
		Exposure: "internal",
		Upstreams: []model.UpstreamTarget{
			{Name: "svc", URL: "http://svc:8080"},
		},
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if route.Host != "internal.localhost" {
		t.Fatalf("route.Host = %q", route.Host)
	}
	if route.Public || route.Protected {
		t.Fatalf("internal route should not be public or protected: %#v", route)
	}
	if route.Exposure != "internal" {
		t.Fatalf("route.Exposure = %q", route.Exposure)
	}
}

func TestAddRejectsDuplicateHost(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	first := model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "one", URL: "http://one:8080"}}}
	if _, err := store.Add(first); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	second := model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "two", URL: "http://two:8080"}}}
	if _, err := store.Add(second); err == nil {
		t.Fatal("second Add() error = nil, want duplicate host error")
	}
}

func TestAddRejectsUnsupportedUpstreamScheme(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	_, err := store.Add(model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "tcp://svc:8080"}}})
	if err == nil || !strings.Contains(err.Error(), "must use http or https") {
		t.Fatalf("Add() error = %v, want http or https scheme error", err)
	}
}

func TestAddAllowsLeftMostWildcardHost(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	route, err := store.Add(model.RouteConfig{Host: "*.Example.COM.", Upstreams: []model.UpstreamTarget{{URL: "http://default:8080"}}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if route.Host != "*.example.com" {
		t.Fatalf("Host = %q, want normalized wildcard", route.Host)
	}
}

func TestAddRejectsInvalidHosts(t *testing.T) {
	for _, host := range []string{"api.*.example.com", "https://example.com", "example.com:443", "localhost", "-bad.example.com"} {
		t.Run(host, func(t *testing.T) {
			store := NewStore("")
			if _, err := store.Add(model.RouteConfig{Host: host, Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}}); err == nil {
				t.Fatalf("Add(%q) error = nil, want host validation error", host)
			}
		})
	}
}

func TestAddNormalizesUpstreamURL(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	route, err := store.Add(model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "svc", URL: " http://svc:8080 "}}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := route.Upstreams[0].URL; got != "http://svc:8080" {
		t.Fatalf("upstream URL = %q, want trimmed URL", got)
	}
}

func TestAddNormalizesRouteSecurity(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	route, err := store.Add(model.RouteConfig{
		Host: "app.localhost",
		Security: model.RouteSecurityConfig{
			AdditionalDeniedMethods:      []string{" trace ", "TRACE"},
			AdditionalDeniedPathPrefixes: []string{" /private/ ", "/private"},
			AllowedCIDRs:                 []string{" 10.0.0.0/8 ", "10.0.0.0/8"},
		},
		Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}},
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if len(route.Security.AdditionalDeniedMethods) != 1 || route.Security.AdditionalDeniedMethods[0] != "TRACE" {
		t.Fatalf("denied methods = %#v", route.Security.AdditionalDeniedMethods)
	}
	if len(route.Security.AdditionalDeniedPathPrefixes) != 1 || route.Security.AdditionalDeniedPathPrefixes[0] != "/private" {
		t.Fatalf("denied paths = %#v", route.Security.AdditionalDeniedPathPrefixes)
	}
	if len(route.Security.AllowedCIDRs) != 1 || route.Security.AllowedCIDRs[0] != "10.0.0.0/8" {
		t.Fatalf("allowed CIDRs = %#v", route.Security.AllowedCIDRs)
	}
}

func TestAddRejectsInvalidRouteSecurity(t *testing.T) {
	tests := []struct {
		name     string
		security model.RouteSecurityConfig
	}{
		{name: "negative body limit", security: model.RouteSecurityConfig{MaxRequestBodyBytes: -1}},
		{name: "invalid method", security: model.RouteSecurityConfig{AdditionalDeniedMethods: []string{"BAD METHOD"}}},
		{name: "invalid path", security: model.RouteSecurityConfig{AdditionalDeniedPathPrefixes: []string{"private/*"}}},
		{name: "invalid CIDR", security: model.RouteSecurityConfig{BlockedCIDRs: []string{"not-a-network"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore("")
			_, err := store.Add(model.RouteConfig{
				Host: "app.localhost", Security: test.security,
				Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}},
			})
			if err == nil {
				t.Fatal("Add() error = nil, want security validation error")
			}
		})
	}
}

func TestLoadPreservesDisabledRoute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	data := []byte(`{"routes":[{"id":"disabled","host":"disabled.localhost","enabled":false,"upstreams":[{"name":"svc","url":"http://svc:8080"}]}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	routes := store.List()
	if len(routes) != 1 {
		t.Fatalf("List() length = %d, want 1", len(routes))
	}
	if routes[0].Enabled {
		t.Fatalf("route.Enabled = true, want false")
	}
}

func TestLoadDefaultsMissingEnabledToTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	data := []byte(`{"routes":[{"id":"legacy","host":"legacy.localhost","upstreams":[{"name":"svc","url":"http://svc:8080"}]}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	routes := store.List()
	if len(routes) != 1 {
		t.Fatalf("List() length = %d, want 1", len(routes))
	}
	if !routes[0].Enabled {
		t.Fatalf("route.Enabled = false, want true")
	}
}

func TestAddRollsBackWhenSaveFails(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Add(model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "http://svc:8080"}}})
	if err == nil {
		t.Fatal("Add() error = nil, want save error")
	}
	if routes := store.List(); len(routes) != 0 {
		t.Fatalf("List() length = %d, want 0", len(routes))
	}
}

func TestSetRuntimeStatusUpdatesLastErrorWithoutPersisting(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	route, err := store.Add(model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "http://svc:8080"}}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	store.SetRuntimeStatus([]model.RouteHealthStatus{{RouteID: route.ID, Host: route.Host, Healthy: false, Error: "connection refused"}})
	if got := store.List()[0].LastError; got != "connection refused" {
		t.Fatalf("LastError = %q", got)
	}
	store.SetRuntimeStatus([]model.RouteHealthStatus{{RouteID: route.ID, Host: route.Host, Healthy: true}})
	if got := store.List()[0].LastError; got != "" {
		t.Fatalf("LastError after healthy status = %q", got)
	}
}

func TestSaveUsesPrivatePermissionsAndRemovesTemporaryFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "routes.json")
	store := NewStore(path)
	if _, err := store.Add(model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "http://svc:8080"}}}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if permission := info.Mode().Perm(); permission != 0o600 {
		t.Fatalf("routes file permission = %o, want 600", permission)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".routes.json.tmp-") {
			t.Fatalf("temporary route file was not removed: %s", entry.Name())
		}
	}
}

func TestAddNormalizesAndValidatesPathPrefix(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	route, err := store.Add(model.RouteConfig{
		Host: "app.localhost", PathPrefix: " /api/ ",
		Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "http://svc:8080"}},
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if route.PathPrefix != "/api" {
		t.Fatalf("PathPrefix = %q, want /api", route.PathPrefix)
	}
	if _, err := store.Add(model.RouteConfig{
		Host: "other.localhost", PathPrefix: "api/*",
		Upstreams: []model.UpstreamTarget{{Name: "svc", URL: "http://svc:8080"}},
	}); err == nil {
		t.Fatal("Add() error = nil, want invalid pathPrefix error")
	}
}

func TestReplaceRejectsDuplicateHostAndPath(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routes.json"))
	first, err := store.Add(model.RouteConfig{Host: "app.localhost", PathPrefix: "/api", Upstreams: []model.UpstreamTarget{{Name: "one", URL: "http://one:8080"}}})
	if err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	second, err := store.Add(model.RouteConfig{Host: "app.localhost", PathPrefix: "/admin", Upstreams: []model.UpstreamTarget{{Name: "two", URL: "http://two:8080"}}})
	if err != nil {
		t.Fatalf("second Add() error = %v", err)
	}
	second.PathPrefix = first.PathPrefix
	if _, err := store.Replace(second); err == nil {
		t.Fatal("Replace() error = nil, want duplicate host and path error")
	}
}

func TestLoadMigratesLegacyRouteIntoRoutingResources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	data := []byte(`{"routes":[{"id":"legacy","host":"app.example.com","pathPrefix":"/api","exposure":"protected","enabled":true,"https":true,"upstreams":[{"name":"app","url":"http://10.0.0.4:8080","healthPath":"/healthz"}]}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	listeners := store.Listeners()
	pools := store.BackendPools()
	rules := store.RoutingRules()
	if len(listeners) != 1 || len(pools) != 1 || len(rules) != 1 {
		t.Fatalf("resources = listeners:%d pools:%d rules:%d, want 1 each", len(listeners), len(pools), len(rules))
	}
	if listeners[0].Hostname != "app.example.com" || listeners[0].Protocol != "https" || listeners[0].Port != 443 {
		t.Fatalf("listener = %#v", listeners[0])
	}
	if len(pools[0].Targets) != 1 || pools[0].Targets[0].Address != "10.0.0.4" {
		t.Fatalf("backend pool = %#v", pools[0])
	}
	if rules[0].BackendPort != 8080 || rules[0].BackendProtocol != "http" || rules[0].HealthPath != "/healthz" {
		t.Fatalf("routing rule = %#v", rules[0])
	}
	compiled := store.List()
	if len(compiled) != 1 || compiled[0].ID != "legacy" || compiled[0].Upstreams[0].URL != "http://10.0.0.4:8080" {
		t.Fatalf("compiled routes = %#v", compiled)
	}
}

func TestRoutingResourcesCompilePersistAndProtectReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	store := NewStore(path)
	listener, err := store.AddListener(model.Listener{Name: "Public HTTPS", Hostname: "app.example.com", Port: 443, Protocol: "https"})
	if err != nil {
		t.Fatalf("AddListener() error = %v", err)
	}
	pool, err := store.AddBackendPool(model.BackendPool{Name: "Application", Targets: []model.BackendTarget{{Address: "10.0.0.4"}, {Address: "app.internal"}}})
	if err != nil {
		t.Fatalf("AddBackendPool() error = %v", err)
	}
	rule, err := store.AddRoutingRule(model.RoutingRule{
		Name: "API", ListenerID: listener.ID, BackendPoolID: pool.ID, BackendPort: 8443,
		BackendProtocol: "https", PathPrefix: "/api", Exposure: "public", Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddRoutingRule() error = %v", err)
	}
	compiled := store.List()
	if len(compiled) != 1 || compiled[0].ListenerPort != 443 || compiled[0].ListenerProtocol != "https" {
		t.Fatalf("compiled route = %#v", compiled)
	}
	if len(compiled[0].Upstreams) != 2 || compiled[0].Upstreams[1].URL != "https://app.internal:8443" {
		t.Fatalf("compiled upstreams = %#v", compiled[0].Upstreams)
	}
	if err := store.DeleteListener(listener.ID); err == nil {
		t.Fatal("DeleteListener() error = nil, want reference error")
	}
	if err := store.DeleteBackendPool(pool.ID); err == nil {
		t.Fatal("DeleteBackendPool() error = nil, want reference error")
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reloaded Load() error = %v", err)
	}
	if len(reloaded.Listeners()) != 1 || len(reloaded.BackendPools()) != 1 || len(reloaded.RoutingRules()) != 1 {
		t.Fatalf("reloaded resources are incomplete")
	}
	if reloaded.RoutingRules()[0].ID != rule.ID || reloaded.List()[0].ID != rule.ID {
		t.Fatalf("reloaded rule id mismatch")
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(persisted), `"version": 2`) || strings.Contains(string(persisted), `"routes"`) {
		t.Fatalf("persisted v2 payload = %s", persisted)
	}
}

func TestCompatibilityRouteMigrationFailureRollsBackResources(t *testing.T) {
	store := NewStore("")
	_, err := store.Add(model.RouteConfig{
		ID: "mixed", Host: "mixed.example.com", Enabled: true, Exposure: "public",
		Upstreams: []model.UpstreamTarget{{URL: "http://app:80"}, {URL: "https://app:443"}},
	})
	if err == nil {
		t.Fatal("Add() error = nil, want mixed-scheme migration error")
	}
	if len(store.Listeners()) != 0 || len(store.BackendPools()) != 0 || len(store.RoutingRules()) != 0 || len(store.List()) != 0 {
		t.Fatalf("failed migration left resources: listeners=%#v pools=%#v rules=%#v routes=%#v", store.Listeners(), store.BackendPools(), store.RoutingRules(), store.List())
	}
}

func TestReplaceResourcesValidatesBeforeAtomicPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	store := NewStore(path)
	listener, err := store.AddListener(model.Listener{Name: "Current", Hostname: "current.example.com", Port: 443, Protocol: "https"})
	if err != nil {
		t.Fatalf("AddListener() error = %v", err)
	}
	pool, err := store.AddBackendPool(model.BackendPool{Name: "Current", Targets: []model.BackendTarget{{Address: "10.0.0.4"}}})
	if err != nil {
		t.Fatalf("AddBackendPool() error = %v", err)
	}
	if _, err := store.AddRoutingRule(model.RoutingRule{Name: "Current", ListenerID: listener.ID, BackendPoolID: pool.ID, BackendPort: 8080, BackendProtocol: "http", Exposure: "public", Enabled: true}); err != nil {
		t.Fatalf("AddRoutingRule() error = %v", err)
	}

	invalid := ResourceSet{
		Version:   ResourceSetVersion,
		Listeners: []model.Listener{{ID: "listener-imported", Name: "Imported", Hostname: "imported.example.com", Port: 443, Protocol: "https"}},
		RoutingRules: []model.RoutingRule{{
			ID: "rule-imported", Name: "Imported", ListenerID: "listener-imported", BackendPoolID: "missing", BackendPort: 8080, BackendProtocol: "http", Exposure: "public", Enabled: true,
		}},
	}
	if err := store.ReplaceResources(invalid); err == nil {
		t.Fatal("ReplaceResources() error = nil, want missing backend pool error")
	}
	if got := store.List(); len(got) != 1 || got[0].Host != "current.example.com" {
		t.Fatalf("failed replacement changed routes = %#v", got)
	}

	valid := ResourceSet{
		Version:      ResourceSetVersion,
		Listeners:    invalid.Listeners,
		BackendPools: []model.BackendPool{{ID: "pool-imported", Name: "Imported", Targets: []model.BackendTarget{{Address: "10.0.0.8"}}}},
		RoutingRules: []model.RoutingRule{{
			ID: "rule-imported", Name: "Imported", ListenerID: "listener-imported", BackendPoolID: "pool-imported", BackendPort: 9090, BackendProtocol: "http", Exposure: "public", Enabled: true, LastError: "stale runtime error",
		}},
	}
	if err := store.ReplaceResources(valid); err != nil {
		t.Fatalf("ReplaceResources() error = %v", err)
	}
	if got := store.List(); len(got) != 1 || got[0].Host != "imported.example.com" || got[0].LastError != "" {
		t.Fatalf("replacement routes = %#v", got)
	}
	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reloaded Load() error = %v", err)
	}
	if got := reloaded.List(); len(got) != 1 || got[0].Host != "imported.example.com" {
		t.Fatalf("persisted replacement routes = %#v", got)
	}
}

func TestStageResourcesWaitsForExplicitPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	store := NewStore(path)
	listener, err := store.AddListener(model.Listener{Name: "Current", Hostname: "current.example.com", Port: 443, Protocol: "https"})
	if err != nil {
		t.Fatalf("AddListener() error = %v", err)
	}
	pool, err := store.AddBackendPool(model.BackendPool{Name: "Current", Targets: []model.BackendTarget{{Address: "10.0.0.4"}}})
	if err != nil {
		t.Fatalf("AddBackendPool() error = %v", err)
	}
	if _, err := store.AddRoutingRule(model.RoutingRule{Name: "Current", ListenerID: listener.ID, BackendPoolID: pool.ID, BackendPort: 8080, BackendProtocol: "http", Exposure: "public", Enabled: true}); err != nil {
		t.Fatalf("AddRoutingRule() error = %v", err)
	}
	staged := ResourceSet{
		Version:      ResourceSetVersion,
		Listeners:    []model.Listener{{ID: "listener-staged", Name: "Staged", Hostname: "staged.example.com", Port: 443, Protocol: "https"}},
		BackendPools: []model.BackendPool{{ID: "pool-staged", Name: "Staged", Targets: []model.BackendTarget{{Address: "10.0.0.8"}}}},
		RoutingRules: []model.RoutingRule{{ID: "rule-staged", Name: "Staged", ListenerID: "listener-staged", BackendPoolID: "pool-staged", BackendPort: 9090, BackendProtocol: "http", Exposure: "public", Enabled: true}},
	}
	if err := store.StageResources(staged); err != nil {
		t.Fatalf("StageResources() error = %v", err)
	}
	if got := store.List(); len(got) != 1 || got[0].Host != "staged.example.com" {
		t.Fatalf("in-memory staged routes = %#v", got)
	}

	beforeApply := NewStore(path)
	if err := beforeApply.Load(); err != nil {
		t.Fatalf("before-apply Load() error = %v", err)
	}
	if got := beforeApply.List(); len(got) != 1 || got[0].Host != "current.example.com" {
		t.Fatalf("routes persisted before Apply = %#v", got)
	}
	if err := store.PersistStaged(); err != nil {
		t.Fatalf("PersistStaged() error = %v", err)
	}
	afterApply := NewStore(path)
	if err := afterApply.Load(); err != nil {
		t.Fatalf("after-apply Load() error = %v", err)
	}
	if got := afterApply.List(); len(got) != 1 || got[0].Host != "staged.example.com" {
		t.Fatalf("routes persisted after Apply = %#v", got)
	}
}
