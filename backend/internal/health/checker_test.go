package health

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestCheckReportsHealthyAndUnhealthyRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ready" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	checker := NewChecker(model.HealthConfig{Enabled: true, TimeoutSeconds: 1, DefaultPath: "/ready"})
	statuses := checker.Check(context.Background(), []model.RouteConfig{
		{ID: "healthy", Host: "healthy.localhost", Enabled: true, Upstreams: []model.UpstreamTarget{{Name: "ok", URL: server.URL}}},
		{ID: "unhealthy", Host: "unhealthy.localhost", Enabled: true, Upstreams: []model.UpstreamTarget{{Name: "bad", URL: server.URL, HealthPath: "/fail"}}},
	})
	if len(statuses) != 2 {
		t.Fatalf("len(statuses) = %d, want 2", len(statuses))
	}
	if !statuses[0].Healthy || statuses[0].Error != "" {
		t.Fatalf("healthy status = %#v", statuses[0])
	}
	if statuses[1].Healthy || statuses[1].Error == "" {
		t.Fatalf("unhealthy status = %#v", statuses[1])
	}
}

func TestCheckBoundsConcurrencyPreservesOrderAndCancels(t *testing.T) {
	var concurrent atomic.Int32
	var peak atomic.Int32
	started := make(chan struct{}, 32)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := concurrent.Add(1)
		defer concurrent.Add(-1)
		for previous := peak.Load(); count > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, count) {
				break
			}
		}
		started <- struct{}{}
		<-request.Context().Done()
	}))
	defer server.Close()
	checker := NewChecker(model.HealthConfig{Enabled: true, TimeoutSeconds: 30})
	routes := make([]model.RouteConfig, 32)
	for index := range routes {
		routes[index] = model.RouteConfig{ID: fmt.Sprint(index), Enabled: true, Upstreams: []model.UpstreamTarget{{Name: "blocked", URL: server.URL}}}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []model.RouteHealthStatus, 1)
	go func() { done <- checker.Check(ctx, routes) }()
	for index := 0; index < maxConcurrentChecks; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("checks did not run concurrently")
		}
	}
	cancel()
	select {
	case statuses := <-done:
		if len(statuses) != len(routes) || peak.Load() != maxConcurrentChecks {
			t.Fatalf("statuses=%d peak=%d", len(statuses), peak.Load())
		}
		for index, status := range statuses {
			if status.RouteID != routes[index].ID || status.Healthy || status.Error == "" {
				t.Fatalf("unexpected status at %d: %#v", index, status)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not stop all probes")
	}
}
