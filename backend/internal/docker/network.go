package docker

import (
	"os"
	"sort"
	"strings"

	"github.com/aidockerfarm/gateway/internal/model"
)

func GatewayContainer(containers []model.ContainerService) (model.ContainerService, bool) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return model.ContainerService{}, false
	}
	for _, container := range containers {
		if container.ID == hostname || strings.HasPrefix(container.ID, hostname) || container.Name == hostname {
			return container, true
		}
	}
	return model.ContainerService{}, false
}

func SharedNetworks(containerNetworks, gatewayNetworks []string) []string {
	gatewaySet := make(map[string]struct{}, len(gatewayNetworks))
	for _, network := range gatewayNetworks {
		gatewaySet[network] = struct{}{}
	}
	shared := make([]string, 0)
	for _, network := range containerNetworks {
		if _, ok := gatewaySet[network]; ok && network != "host" && network != "none" {
			shared = append(shared, network)
		}
	}
	sort.Strings(shared)
	return shared
}

func SharedNetworkAddress(container model.ContainerService, gatewayNetworks []string) string {
	shared := SharedNetworks(container.Networks, gatewayNetworks)
	for _, network := range shared {
		for _, endpoint := range container.NetworkEndpoints {
			if endpoint.Name == network && strings.TrimSpace(endpoint.Address) != "" {
				return strings.TrimSpace(endpoint.Address)
			}
		}
	}
	for _, network := range shared {
		if network != "bridge" {
			if name := strings.TrimSpace(container.Labels["com.docker.compose.service"]); name != "" {
				return name
			}
			return container.Name
		}
	}
	return ""
}
