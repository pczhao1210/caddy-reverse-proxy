package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestDiscoverUsesHTTPEndpointAndHealthPathLabel(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[
			{"Id":%q,"Names":["/gateway"],"Labels":{"caddy.enable":"true","caddy.host":"gateway.localhost","caddy.port":"8080"},"NetworkSettings":{"Networks":{"bridge":{"IPAddress":"172.17.0.2"}}}},
			{
				"Id":"abcdef1234567890",
				"Names":["/web"],
				"Image":"example/web:latest",
				"Labels":{"caddy.enable":"true","caddy.host":"web.localhost","caddy.port":"8080","caddy.health_path":"/ready"},
				"State":"running",
				"Status":"Up 1 second",
				"Ports":[{"PrivatePort":8080,"Type":"tcp"}],
				"NetworkSettings":{"Networks":{"a-private":{"IPAddress":"10.0.0.9"},"bridge":{"IPAddress":"172.17.0.3"}}}
			}
		]`, hostname)
	}))
	defer server.Close()

	discoverer := NewDiscoverer(model.DockerConfig{Enabled: true, Endpoint: server.URL}, nil)
	containers, routes, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(containers) != 2 || len(routes) != 1 {
		t.Fatalf("containers=%d routes=%d, want 2 and 1", len(containers), len(routes))
	}
	if routes[0].Upstreams[0].HealthPath != "/ready" {
		t.Fatalf("HealthPath = %q", routes[0].Upstreams[0].HealthPath)
	}
	if routes[0].Upstreams[0].URL != "http://172.17.0.3:8080" {
		t.Fatalf("upstream URL = %q", routes[0].Upstreams[0].URL)
	}
}

func TestToServiceDeduplicatesContainerPorts(t *testing.T) {
	service := toService(containerPayload{
		Names: []string{"/web"},
		Ports: []portPayload{
			{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
			{PrivatePort: 80, PublicPort: 0, Type: "TCP"},
			{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		},
	})

	if len(service.Ports) != 2 {
		t.Fatalf("ports = %#v, want two unique container ports", service.Ports)
	}
	if service.Ports[0].PrivatePort != 80 || service.Ports[0].PublicPort != 8080 || service.Ports[0].Type != "tcp" {
		t.Fatalf("first port = %#v, want normalized 80/tcp mapping", service.Ports[0])
	}
}

func TestLabelRoutesRequireReachableSharedNetwork(t *testing.T) {
	var payload containerPayload
	if err := json.Unmarshal([]byte(`{"Names":["/web"],"Labels":{"caddy.enable":"true","caddy.host":"web.localhost","caddy.port":"8080","com.docker.compose.service":"web"},"NetworkSettings":{"Networks":{"z-shared":{"IPAddress":"10.2.0.4"},"a-private":{"IPAddress":"10.1.0.4"}}}}`), &payload); err != nil {
		t.Fatal(err)
	}
	service := toService(payload)
	for _, networks := range [][]string{nil, {"other"}, {"host"}, {"none"}} {
		if _, ok := routeFromLabels(service, networks); ok {
			t.Fatalf("unreachable route generated for %v", networks)
		}
	}
	route, ok := routeFromLabels(service, []string{"z-shared"})
	if !ok || route.Upstreams[0].URL != "http://10.2.0.4:8080" {
		t.Fatalf("route selected private network: %#v", route)
	}
	if address := SharedNetworkAddress(service, []string{"z-shared", "a-private"}); address != "10.1.0.4" {
		t.Fatalf("non-deterministic address: %s", address)
	}
	service.NetworkEndpoints = nil
	if address := SharedNetworkAddress(service, []string{"z-shared"}); address != "web" {
		t.Fatalf("Compose DNS fallback: %s", address)
	}
	service.Networks = []string{"bridge"}
	if address := SharedNetworkAddress(service, []string{"bridge"}); address != "" {
		t.Fatalf("default bridge cannot resolve service names: %s", address)
	}
}

func TestUnknownGatewaySkipsAutomaticRoutesWithDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, `[{"Id":"unrelated-container","Names":["/web"],"Labels":{"caddy.enable":"true","caddy.host":"web.localhost","caddy.port":"8080"}}]`)
	}))
	defer server.Close()
	discoverer := NewDiscoverer(model.DockerConfig{Enabled: true, Endpoint: server.URL}, nil)
	containers, routes, err := discoverer.Discover(context.Background())
	if err != nil || len(routes) != 0 || len(containers) != 1 || containers[0].RouteWarning == "" {
		t.Fatalf("containers=%v routes=%v err=%v", containers, routes, err)
	}
}
