package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
)

type Discoverer struct {
	cfg     model.DockerConfig
	client  *http.Client
	baseURL string
	logger  *slog.Logger
}

type containerPayload struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	Labels          map[string]string `json:"Labels"`
	State           string            `json:"State"`
	Status          string            `json:"Status"`
	Ports           []portPayload     `json:"Ports"`
	NetworkSettings struct {
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

type networkPayload struct {
	IPAddress string `json:"IPAddress"`
}

type portPayload struct {
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

func NewDiscoverer(cfg model.DockerConfig, logger *slog.Logger) *Discoverer {
	if cfg.Endpoint != "" {
		return &Discoverer{
			cfg:     cfg,
			client:  &http.Client{Timeout: 10 * time.Second},
			baseURL: strings.TrimRight(cfg.Endpoint, "/"),
			logger:  logger,
		}
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", cfg.SocketPath)
		},
	}
	return &Discoverer{
		cfg: cfg,
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		baseURL: "http://docker",
		logger:  logger,
	}
}

func (d *Discoverer) Discover(ctx context.Context) ([]model.ContainerService, []model.RouteConfig, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/containers/json?all=false", nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := d.client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("docker list containers: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("docker list containers returned %s", response.Status)
	}

	var payload []containerPayload
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, nil, err
	}

	services := make([]model.ContainerService, 0, len(payload))
	routeList := make([]model.RouteConfig, 0)
	for _, container := range payload {
		service := toService(container)
		if route, ok := routeFromLabels(service); ok {
			service.RouteHint = &route
			routeList = append(routeList, route)
		}
		services = append(services, service)
	}
	sort.Slice(services, func(left int, right int) bool { return services[left].Name < services[right].Name })
	return services, routeList, nil
}

func toService(container containerPayload) model.ContainerService {
	ports := make([]model.ContainerPort, 0, len(container.Ports))
	for _, port := range container.Ports {
		ports = append(ports, model.ContainerPort{PrivatePort: port.PrivatePort, PublicPort: port.PublicPort, Type: port.Type})
	}
	networks := make([]string, 0, len(container.NetworkSettings.Networks))
	addresses := make([]string, 0, len(container.NetworkSettings.Networks))
	for name, raw := range container.NetworkSettings.Networks {
		networks = append(networks, name)
		var network networkPayload
		if err := json.Unmarshal(raw, &network); err == nil && strings.TrimSpace(network.IPAddress) != "" {
			addresses = append(addresses, network.IPAddress)
		}
	}
	sort.Strings(networks)
	sort.Strings(addresses)
	return model.ContainerService{ID: container.ID, Name: cleanName(container.Names), Image: container.Image, State: container.State, Status: container.Status, Labels: container.Labels, Ports: ports, Networks: networks, NetworkAddresses: addresses}
}

func routeFromLabels(service model.ContainerService) (model.RouteConfig, bool) {
	labels := service.Labels
	if !labelTrue(labels["caddy.enable"]) {
		return model.RouteConfig{}, false
	}
	host := strings.TrimSpace(labels["caddy.host"])
	if host == "" {
		return model.RouteConfig{}, false
	}
	port := labels["caddy.port"]
	if port == "" && len(service.Ports) > 0 {
		port = strconv.Itoa(service.Ports[0].PrivatePort)
	}
	if port == "" {
		return model.RouteConfig{}, false
	}
	exposure := strings.ToLower(labels["exposure.mode"])
	if exposure == "" {
		exposure = "public"
	}
	public := exposure == "" || exposure == "public" || exposure == "protected"
	protected := exposure == "protected"
	upstreamAddress := serviceUpstreamAddress(service)
	healthPath := strings.TrimSpace(labels["caddy.health_path"])
	return model.RouteConfig{
		ID:         "docker-" + shortID(service.ID),
		Host:       strings.ToLower(host),
		Exposure:   exposure,
		Enabled:    true,
		Public:     public,
		HTTPS:      true,
		WebSocket:  labelTrue(labels["caddy.websocket"]),
		Protected:  protected,
		Source:     "docker",
		Discovered: true,
		Upstreams:  []model.UpstreamTarget{{Name: upstreamAddress, URL: fmt.Sprintf("http://%s:%s", upstreamAddress, port), HealthPath: healthPath}},
	}, true
}

func serviceUpstreamAddress(service model.ContainerService) string {
	if len(service.NetworkAddresses) > 0 {
		return service.NetworkAddresses[0]
	}
	if name := strings.TrimSpace(service.Labels["com.docker.compose.service"]); name != "" {
		return name
	}
	return service.Name
}

func cleanName(names []string) string {
	if len(names) == 0 {
		return "unknown"
	}
	return strings.TrimPrefix(names[0], "/")
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func labelTrue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
