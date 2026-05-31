package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
