package reconcile

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

type runtimeConfigLoader struct{ testLoader }

func (runtimeConfigLoader) Config(context.Context) ([]byte, error) {
	return []byte(`{"active":true}`), nil
}

func TestRuntimeInspectionHoldsConfigurationLock(t *testing.T) {
	controller := New(Options{Loader: runtimeConfigLoader{}})
	if err := controller.WithRuntimeConfig(context.Background(), func(data []byte) error {
		if controller.syncMu.TryLock() {
			controller.syncMu.Unlock()
			t.Fatal("runtime inspection did not lock configuration commits")
		}
		if string(data) != `{"active":true}` {
			t.Fatalf("unexpected active config: %s", data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !controller.syncMu.TryLock() {
		t.Fatal("configuration lock leaked")
	}
	controller.syncMu.Unlock()
	controller.loader = testLoader{}
	if err := controller.WithRuntimeConfig(context.Background(), func([]byte) error { t.Fatal("unknown runtime accepted"); return nil }); err == nil {
		t.Fatal("missing reader accepted")
	}
}

func (testLoader) Load(context.Context, []byte) error {
	return nil
}

type callbackLoader func() error

func (loader callbackLoader) Load(context.Context, []byte) error {
	return loader()
}

func TestApplyConfirmsOnlyLoadedRevision(t *testing.T) {
	store := routes.NewStore(filepath.Join(t.TempDir(), "routes.json"))
	renderer := &captureRenderer{}
	loader := callbackLoader(func() error {
		_, err := store.Add(model.RouteConfig{Host: "new.example.com", Enabled: true, Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}})
		return err
	})
	controller := New(Options{Store: store, Renderer: renderer, Loader: loader})
	result := controller.Sync(context.Background())
	if result.Error != "" || !controller.RoutingChangesPending() || len(store.AppliedSnapshot().Routes) != 0 || len(store.List()) != 1 {
		t.Fatalf("apply lost newer draft: result=%+v pending=%t", result, controller.RoutingChangesPending())
	}
}

func TestBackgroundSyncAfterRestartUsesAppliedSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	store := routes.NewStore(path)
	created, err := store.Add(model.RouteConfig{Host: "live.example.com", Enabled: true, Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkApplied(store.DesiredSnapshot()); err != nil {
		t.Fatal(err)
	}
	created.Host = "draft.example.com"
	if _, err := store.Replace(created); err != nil {
		t.Fatal(err)
	}
	restarted := routes.NewStore(path)
	if err := restarted.Load(); err != nil {
		t.Fatal(err)
	}
	renderer := &captureRenderer{}
	controller := New(Options{Store: restarted, Renderer: renderer, Loader: testLoader{}})
	if result := controller.SyncWithoutPendingRoutingChanges(context.Background()); result.Error != "" {
		t.Fatal(result.Error)
	}
	if len(renderer.routes) != 1 || renderer.routes[0].Host != "live.example.com" || !controller.RoutingChangesPending() {
		t.Fatalf("background applied draft: %#v", renderer.routes)
	}
}

func TestApplyPersistenceFailureRestoresRuntime(t *testing.T) {
	store := routes.NewStore("")
	if _, err := store.Add(model.RouteConfig{Host: "draft.example.com", Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}}); err != nil {
		t.Fatal(err)
	}
	renderer := &captureRenderer{}
	loads := 0
	controller := New(Options{Store: store, Renderer: renderer, Loader: callbackLoader(func() error { loads++; return nil })})
	result := controller.SyncWithCommit(context.Background(), nil, func(routes.Snapshot) error { return errors.New("disk unavailable") })
	if result.Error == "" || result.CaddyLoaded || loads != 2 || len(renderer.routes) != 0 || !store.Pending() {
		t.Fatalf("failed commit did not restore previous runtime: result=%+v loads=%d", result, loads)
	}
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

type callbackHealthChecker func(context.Context, []model.RouteConfig) []model.RouteHealthStatus

func (checker callbackHealthChecker) Check(ctx context.Context, routes []model.RouteConfig) []model.RouteHealthStatus {
	return checker(ctx, routes)
}

func TestSlowHealthDoesNotBlockApplyOrPublishStaleStatus(t *testing.T) {
	store := routes.NewStore("")
	route, err := store.Add(model.RouteConfig{Host: "app.localhost", Upstreams: []model.UpstreamTarget{{URL: "http://app:8080"}}})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var calls atomic.Int32
	checker := callbackHealthChecker(func(ctx context.Context, routes []model.RouteConfig) []model.RouteHealthStatus {
		message := "new probe"
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			message = "stale probe"
			if ctx.Err() == nil {
				t.Error("old probe context was not canceled")
			}
		}
		return []model.RouteHealthStatus{{RouteID: route.ID, Error: message}}
	})
	controller := New(Options{Store: store, Renderer: testRenderer{}, Loader: testLoader{}, HealthChecker: checker})
	first := make(chan model.ReconcileResult, 1)
	go func() { first <- controller.Sync(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first probe did not start")
	}
	second := make(chan model.ReconcileResult, 1)
	go func() { second <- controller.Sync(context.Background()) }()
	select {
	case result := <-second:
		if result.Error != "" || !result.CaddyLoaded {
			t.Fatalf("new apply failed: %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("health probe blocked configuration apply")
	}
	unblock()
	<-first
	if controller.Last().RouteHealth[0].Error != "new probe" || store.List()[0].LastError != "new probe" {
		t.Fatal("stale health result replaced current status")
	}
}

type failingDiscoverer struct{}

func (failingDiscoverer) Discover(context.Context) ([]model.ContainerService, []model.RouteConfig, error) {
	return nil, nil, errors.New("docker unavailable")
}

type intermittentDiscoverer struct {
	calls int
}

func (d *intermittentDiscoverer) Discover(context.Context) ([]model.ContainerService, []model.RouteConfig, error) {
	d.calls++
	if d.calls > 1 {
		return nil, nil, errors.New("docker unavailable")
	}
	return nil, []model.RouteConfig{{ID: "discovered", Host: "discovered.localhost", Enabled: true, Public: true, Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}}}}, nil
}

type serialLoader struct {
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
	active  int
	max     int
}

func (l *serialLoader) Load(context.Context, []byte) error {
	l.mu.Lock()
	l.active++
	if l.active > l.max {
		l.max = l.active
	}
	l.mu.Unlock()
	l.entered <- struct{}{}
	<-l.release
	l.mu.Lock()
	l.active--
	l.mu.Unlock()
	return nil
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

func TestSyncKeepsLastSuccessfulDiscoveredRoutes(t *testing.T) {
	discoverer := &intermittentDiscoverer{}
	renderer := &captureRenderer{}
	reconciler := New(Options{
		Config:     model.AppConfig{},
		Store:      routes.NewStore(""),
		Discoverer: discoverer,
		Renderer:   renderer,
		Loader:     testLoader{},
	})

	first := reconciler.Sync(context.Background())
	second := reconciler.Sync(context.Background())
	if first.DiscoveredRoutes != 1 || second.DiscoveredRoutes != 1 {
		t.Fatalf("discovered routes = first %d second %d, want 1 and 1", first.DiscoveredRoutes, second.DiscoveredRoutes)
	}
	if len(renderer.routes) != 1 || renderer.routes[0].ID != "discovered" {
		t.Fatalf("rendered routes after discovery failure = %#v", renderer.routes)
	}
}

func TestSyncWithoutPendingRoutingChangesUsesLastAppliedRoutes(t *testing.T) {
	store := routes.NewStore("")
	created, err := store.Add(model.RouteConfig{Host: "live.localhost", Enabled: true, Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	renderer := &captureRenderer{}
	reconciler := New(Options{
		Config:   model.AppConfig{},
		Store:    store,
		Renderer: renderer,
		Loader:   testLoader{},
	})
	if result := reconciler.Sync(context.Background()); result.Error != "" {
		t.Fatalf("initial Sync() error = %q", result.Error)
	}
	_, err = store.Replace(model.RouteConfig{ID: created.ID, Host: "draft.localhost", Enabled: true, Upstreams: []model.UpstreamTarget{{Name: "app", URL: "http://app:8080"}}})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if result := reconciler.SyncWithoutPendingRoutingChanges(context.Background()); result.Error != "" {
		t.Fatalf("SyncWithoutPendingRoutingChanges() error = %q", result.Error)
	}
	if len(renderer.routes) != 1 || renderer.routes[0].Host != "live.localhost" {
		t.Fatalf("routes after preserved sync = %#v, want live.localhost", renderer.routes)
	}

	if result := reconciler.Sync(context.Background()); result.Error != "" {
		t.Fatalf("manual Sync() error = %q", result.Error)
	}
	if len(renderer.routes) != 1 || renderer.routes[0].Host != "draft.localhost" {
		t.Fatalf("routes after manual sync = %#v, want draft.localhost", renderer.routes)
	}
}

func TestSyncSerializesConcurrentLoads(t *testing.T) {
	loader := &serialLoader{entered: make(chan struct{}, 2), release: make(chan struct{})}
	reconciler := New(Options{
		Config:   model.AppConfig{},
		Store:    routes.NewStore(""),
		Renderer: testRenderer{},
		Loader:   loader,
	})

	done := make(chan struct{}, 2)
	go func() {
		reconciler.Sync(context.Background())
		done <- struct{}{}
	}()
	<-loader.entered
	go func() {
		reconciler.Sync(context.Background())
		done <- struct{}{}
	}()

	select {
	case <-loader.entered:
		t.Fatal("second Caddy load started before the first completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(loader.release)
	<-done
	<-done

	loader.mu.Lock()
	defer loader.mu.Unlock()
	if loader.max != 1 {
		t.Fatalf("maximum concurrent loads = %d, want 1", loader.max)
	}
}
