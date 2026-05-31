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
