package api

import (
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestContainerBindPolicySharedNetworkAllowsBind(t *testing.T) {
	container := model.ContainerService{
		Name:     "app",
		Networks: []string{"bridge", "proxy-net"},
		Ports:    []model.ContainerPort{{PrivatePort: 8080, Type: "tcp"}},
		NetworkEndpoints: []model.NetworkEndpoint{
			{Name: "bridge", Address: "172.17.0.5"},
			{Name: "proxy-net", Address: "172.22.0.5"},
		},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 8080)
	if !policy.CanBind {
		t.Fatalf("CanBind = false, want true")
	}
	if policy.Mode != "shared-network" {
		t.Fatalf("Mode = %q, want shared-network", policy.Mode)
	}
	if upstream := bindUpstreamName(container, []string{"proxy-net"}); upstream != "172.22.0.5" {
		t.Fatalf("bindUpstreamName = %q, want 172.22.0.5", upstream)
	}
}

func TestContainerBindPolicyHostNetworkSuggestsExplicitRoute(t *testing.T) {
	container := model.ContainerService{
		Name:     "portainer",
		Networks: []string{"host"},
		Ports:    []model.ContainerPort{{PrivatePort: 9443, Type: "tcp"}},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 9443)
	if policy.CanBind {
		t.Fatalf("CanBind = true, want false")
	}
	if policy.Mode != "host-network" {
		t.Fatalf("Mode = %q, want host-network", policy.Mode)
	}
	if policy.SuggestedUpstream != "http://host.docker.internal:9443" {
		t.Fatalf("SuggestedUpstream = %q", policy.SuggestedUpstream)
	}
}

func TestContainerBindPolicyBridgeWithoutSharedNetworkRequiresAttach(t *testing.T) {
	container := model.ContainerService{
		Name:             "bridge-only",
		Networks:         []string{"bridge"},
		Ports:            []model.ContainerPort{{PrivatePort: 8080, Type: "tcp"}},
		NetworkEndpoints: []model.NetworkEndpoint{{Name: "bridge", Address: "172.17.0.8"}},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 8080)
	if policy.CanBind {
		t.Fatalf("CanBind = true, want false")
	}
	if policy.Mode != "bridge-unreachable" {
		t.Fatalf("Mode = %q, want bridge-unreachable", policy.Mode)
	}
}

func TestContainerBindPolicyPublishedPortSuggestsHostGateway(t *testing.T) {
	container := model.ContainerService{
		Name:     "published-app",
		Networks: []string{"bridge"},
		Ports:    []model.ContainerPort{{PrivatePort: 8080, PublicPort: 18080, Type: "tcp"}},
	}

	policy := containerBindPolicy(container, []string{"proxy-net"}, 8080)
	if policy.Mode != "published-port" {
		t.Fatalf("Mode = %q, want published-port", policy.Mode)
	}
	if policy.SuggestedUpstream != "http://host.docker.internal:18080" {
		t.Fatalf("SuggestedUpstream = %q", policy.SuggestedUpstream)
	}
}
