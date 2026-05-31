package reconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
)

type testRenderer struct{}

func (testRenderer) Render([]model.RouteConfig) ([]byte, error) {
	return []byte(`{}`), nil
}

type captureRenderer struct {
	routes []model.RouteConfig
}

func (r *captureRenderer) Render(routes []model.RouteConfig) ([]byte, error) {
	r.routes = append([]model.RouteConfig{}, routes...)
	return []byte(`{}`), nil
}

type testLoader struct{}

func (testLoader) Load(context.Context, []byte) error {
	return nil
}

type testAzureManager struct {
	routes []model.RouteConfig
}

func (m *testAzureManager) Reconcile(_ context.Context, routes []model.RouteConfig) model.AzureResult {
	m.routes = append([]model.RouteConfig{}, routes...)
	return model.AzureResult{Enabled: true}
}

type testHealthChecker struct{}

func (testHealthChecker) Check(_ context.Context, routes []model.RouteConfig) []model.RouteHealthStatus {
	statuses := make([]model.RouteHealthStatus, 0, len(routes))
	for _, route := range routes {
		statuses = append(statuses, model.RouteHealthStatus{RouteID: route.ID, Host: route.Host, Healthy: false, Error: "not ready"})
	}
	return statuses
}

type failingDiscoverer struct{}

func (failingDiscoverer) Discover(context.Context) ([]model.ContainerService, []model.RouteConfig, error) {
	return nil, nil, errors.New("docker unavailable")
}

func TestSyncReturnsFinalizedResult(t *testing.T) {
	reconciler := New(Options{
		Config:   model.AppConfig{},
		Store:    routes.NewStore(""),
		Renderer: testRenderer{},
		Loader:   testLoader{},
	})
	result := reconciler.Sync(context.Background())
	if result.FinishedAt.IsZero() {
		t.Fatal("FinishedAt is zero")
	}
	if result.Duration < 0 {
		t.Fatalf("Duration = %s, want non-negative", result.Duration)
	}
	if !result.CaddyLoaded {
		t.Fatal("CaddyLoaded = false, want true")
	}
}

func TestSyncRunsHealthChecksAndUpdatesRouteStatus(t *testing.T) {
	store := routes.NewStore("")
	route, err := store.Add(model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	reconciler := New(Options{
		Config:        model.AppConfig{},
		Store:         store,
		Renderer:      testRenderer{},
		Loader:        testLoader{},
		HealthChecker: testHealthChecker{},
	})

	result := reconciler.Sync(context.Background())
	if result.HealthChecks != 1 || result.UnhealthyRoutes != 1 {
		t.Fatalf("health result = checks %d unhealthy %d", result.HealthChecks, result.UnhealthyRoutes)
	}
	if len(result.RouteHealth) != 1 || result.RouteHealth[0].RouteID != route.ID {
		t.Fatalf("RouteHealth = %#v", result.RouteHealth)
	}
	if got := store.List()[0].LastError; got != "not ready" {
		t.Fatalf("store LastError = %q", got)
	}
}

func TestSyncIncludesManagementHostForAzureReconcile(t *testing.T) {
	azureManager := &testAzureManager{}
	reconciler := New(Options{
		Config:       model.AppConfig{Control: model.ControlConfig{ManagementHost: "admin.example.com"}},
		Store:        routes.NewStore(""),
		Renderer:     testRenderer{},
		Loader:       testLoader{},
		AzureManager: azureManager,
	})
	result := reconciler.Sync(context.Background())
	if result.Error != "" {
		t.Fatalf("Sync() error = %s", result.Error)
	}
	if len(azureManager.routes) != 1 {
		t.Fatalf("azure route count = %d, want 1", len(azureManager.routes))
	}
	route := azureManager.routes[0]
	if route.Host != "admin.example.com" || !route.Public || !route.Protected || route.Exposure != "protected" {
		t.Fatalf("management azure route = %#v", route)
	}
}

func TestSyncAppliesExplicitRoutesWhenDiscoveryFails(t *testing.T) {
	store := routes.NewStore("")
	created, err := store.Add(model.RouteConfig{Host: "app.localhost", Enabled: true, Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	renderer := &captureRenderer{}
	reconciler := New(Options{
		Config:     model.AppConfig{},
		Store:      store,
		Discoverer: failingDiscoverer{},
		Renderer:   renderer,
		Loader:     testLoader{},
	})

	result := reconciler.Sync(context.Background())
	if result.Error != "" {
		t.Fatalf("Sync() error = %q, want explicit routes to continue", result.Error)
	}
	if !result.CaddyLoaded || result.AppliedRoutes != 1 {
		t.Fatalf("Sync() loaded=%v applied=%d, want loaded explicit route", result.CaddyLoaded, result.AppliedRoutes)
	}
	if len(renderer.routes) != 1 || renderer.routes[0].ID != created.ID {
		t.Fatalf("rendered routes = %#v, want explicit route %q", renderer.routes, created.ID)
	}
}
