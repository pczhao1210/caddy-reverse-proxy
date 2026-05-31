package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestDiscoverUsesHTTPEndpointAndHealthPathLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"Id":"abcdef1234567890",
				"Names":["/web"],
				"Image":"example/web:latest",
				"Labels":{"caddy.enable":"true","caddy.host":"web.localhost","caddy.port":"8080","caddy.health_path":"/ready"},
				"State":"running",
				"Status":"Up 1 second",
				"Ports":[{"PrivatePort":8080,"Type":"tcp"}],
				"NetworkSettings":{"Networks":{"bridge":{"IPAddress":"172.17.0.3"}}}
			}
		]`))
	}))
	defer server.Close()

	discoverer := NewDiscoverer(model.DockerConfig{Enabled: true, Endpoint: server.URL}, nil)
	containers, routes, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(containers) != 1 || len(routes) != 1 {
		t.Fatalf("containers=%d routes=%d, want 1 and 1", len(containers), len(routes))
	}
	if routes[0].Upstreams[0].HealthPath != "/ready" {
		t.Fatalf("HealthPath = %q", routes[0].Upstreams[0].HealthPath)
	}
	if routes[0].Upstreams[0].URL != "http://172.17.0.3:8080" {
		t.Fatalf("upstream URL = %q", routes[0].Upstreams[0].URL)
	}
}
