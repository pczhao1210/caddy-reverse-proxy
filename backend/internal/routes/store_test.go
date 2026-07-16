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
