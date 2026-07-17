package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/aidockerfarm/gateway/internal/audit"
	"github.com/aidockerfarm/gateway/internal/auth"
	"github.com/aidockerfarm/gateway/internal/azure"
	"github.com/aidockerfarm/gateway/internal/certificate"
	appconfig "github.com/aidockerfarm/gateway/internal/config"
	"github.com/aidockerfarm/gateway/internal/logs"
	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
	uiassets "github.com/aidockerfarm/gateway/internal/ui"
)

type Discoverer interface {
	Discover(context.Context) ([]model.ContainerService, []model.RouteConfig, error)
}

type Reconciler interface {
	Sync(context.Context) model.ReconcileResult
	SyncWithoutPendingRoutingChanges(context.Context) model.ReconcileResult
	Last() model.ReconcileResult
}

type routingChangeController interface {
	MarkRoutingChangesPending()
	ClearRoutingChangesPending()
	RoutingChangesPending() bool
}

type RuntimeStatus interface {
	Ready() bool
	LastError() error
}

type certificateUpdater interface {
	UpdateCertificate(model.CertificateConfig)
}

type runtimeConfigUpdater interface {
	UpdateConfig(model.AppConfig)
}

type CertificateStore interface {
	Save(model.CertificateConfig) error
}

type CertificateInspector interface {
	Inspect(float64) (certificate.Snapshot, error)
}

type SettingsStore interface {
	Load() (appconfig.Settings, bool, error)
	Save(appconfig.Settings) error
}

type AzurePermissionChecker interface {
	Check(context.Context, model.AzureConfig) (azure.PermissionCheckResult, error)
}

type RuntimeLogStore interface {
	ReadLast(int) []logs.Entry
}

type Options struct {
	Config                 model.AppConfig
	Store                  *routes.Store
	Discoverer             Discoverer
	Reconciler             Reconciler
	Runtime                RuntimeStatus
	AuditLog               AuditLog
	CertificateStore       CertificateStore
	CertificateInspector   CertificateInspector
	SettingsStore          SettingsStore
	AzurePermissionChecker AzurePermissionChecker
	RuntimeLogs            RuntimeLogStore
	Logger                 *slog.Logger
}

type Server struct {
	mu                     sync.RWMutex
	configurationMu        sync.Mutex
	cfg                    model.AppConfig
	configurationDraft     *configurationImportDraft
	store                  *routes.Store
	discoverer             Discoverer
	reconciler             Reconciler
	runtime                RuntimeStatus
	auditLog               AuditLog
	certificateStore       CertificateStore
	certificateInspector   CertificateInspector
	settingsStore          SettingsStore
	azurePermissionChecker AzurePermissionChecker
	runtimeLogs            RuntimeLogStore
	logger                 *slog.Logger
}

type AuditLog interface {
	Record(context.Context, string, map[string]any) error
	ReadLast(int) ([]audit.Event, error)
}

type dockerStatus struct {
	Enabled     bool     `json:"enabled"`
	Active      bool     `json:"active"`
	Profile     string   `json:"profile"`
	SocketPath  string   `json:"socketPath,omitempty"`
	Endpoint    string   `json:"endpoint,omitempty"`
	Reason      string   `json:"reason"`
	NextActions []string `json:"nextActions,omitempty"`
}

type bindContainerRequest struct {
	ContainerID string `json:"containerId"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Scheme      string `json:"scheme"`
	Exposure    string `json:"exposure"`
	HTTPS       bool   `json:"https"`
	WebSocket   bool   `json:"websocket"`
}

func NewServer(options Options) *Server {
	return &Server{cfg: options.Config, store: options.Store, discoverer: options.Discoverer, reconciler: options.Reconciler, runtime: options.Runtime, auditLog: options.AuditLog, certificateStore: options.CertificateStore, certificateInspector: options.CertificateInspector, settingsStore: options.SettingsStore, azurePermissionChecker: options.AzurePermissionChecker, runtimeLogs: options.RuntimeLogs, logger: options.Logger}
}

func (s *Server) Handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("/healthz", s.handleReady)
	root.HandleFunc("/livez", s.handleLive)
	root.HandleFunc("/readyz", s.handleReady)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/status", s.handleStatus)
	apiMux.HandleFunc("/api/listeners", s.handleListeners)
	apiMux.HandleFunc("/api/listeners/", s.handleListenerByID)
	apiMux.HandleFunc("/api/backend-pools", s.handleBackendPools)
	apiMux.HandleFunc("/api/backend-pools/", s.handleBackendPoolByID)
	apiMux.HandleFunc("/api/routing-rules", s.handleRoutingRules)
	apiMux.HandleFunc("/api/routing-rules/", s.handleRoutingRuleByID)
	apiMux.HandleFunc("/api/routes", s.handleRoutes)
	apiMux.HandleFunc("/api/routes/", s.handleRouteByID)
	apiMux.HandleFunc("/api/discovery/bind", s.handleBindContainer)
	apiMux.HandleFunc("/api/discovery/containers", s.handleContainers)
	apiMux.HandleFunc("/api/reconcile", s.handleReconcile)
	apiMux.HandleFunc("/api/certificate", s.handleCertificate)
	apiMux.HandleFunc("/api/certificate/refresh", s.handleCertificateRefresh)
	apiMux.HandleFunc("/api/config", s.handleConfig)
	apiMux.HandleFunc("/api/settings", s.handleSettings)
	apiMux.HandleFunc("/api/settings/configuration", s.handleConfigurationBundle)
	apiMux.HandleFunc("/api/settings/security", s.handleSecuritySettings)
	apiMux.HandleFunc("/api/settings/system", s.handleSystemSettings)
	apiMux.HandleFunc("/api/settings/azure/permissions", s.handleAzurePermissions)
	apiMux.HandleFunc("/api/audit", s.handleAudit)
	apiMux.HandleFunc("/api/logs", s.handleLogs)
	root.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Middleware(s.configSnapshot().Auth, apiMux).ServeHTTP(w, r)
	}))

	root.Handle("/", uiassets.Handler())
	return root
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "alive", "profile": s.configSnapshot().Profile})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	response := map[string]any{"status": "not_ready", "profile": s.configSnapshot().Profile, "caddyReady": false}
	if s.runtime == nil {
		response["error"] = "caddy runtime status is unavailable"
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	if !s.runtime.Ready() {
		if err := s.runtime.LastError(); err != nil {
			response["error"] = err.Error()
		}
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	response["status"] = "ready"
	response["caddyReady"] = true
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	cfg := s.configSnapshot()
	certificatePolicy := s.desiredCertificatePolicy(cfg.Gateway.Certificate)
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":                    cfg.Profile,
		"deploymentMode":             cfg.DeploymentMode,
		"listeners":                  s.store.Listeners(),
		"backendPools":               s.store.BackendPools(),
		"routingRules":               s.store.RoutingRules(),
		"routes":                     s.store.List(),
		"routingChangesPending":      s.routingChangesPending(),
		"configurationImportPending": s.configurationImportPending(),
		"lastReconcile":              s.reconciler.Last(),
		"azure":                      azure.StatusForConfig(cfg),
		"docker":                     s.dockerStatus(),
		"certificate":                certificateStatusWithPolicy(cfg, certificatePolicy),
		"security":                   cfg.Security,
		"health":                     cfg.Health,
		"audit":                      map[string]any{"enabled": cfg.Audit.Enabled, "file": cfg.Audit.File},
	})
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"routes": s.store.List()})
	case http.MethodPost:
		var route model.RouteConfig
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := s.store.Add(route)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.audit("route.create", map[string]any{"routeId": created.ID, "host": created.Host, "exposure": created.Exposure})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusCreated, map[string]any{"route": created, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleRouteByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/routes/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "route id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var route model.RouteConfig
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		route.ID = id
		updated, err := s.store.Replace(route)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.audit("route.update", map[string]any{"routeId": updated.ID, "host": updated.Host, "exposure": updated.Exposure})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"route": updated, "pendingApply": true})
	case http.MethodDelete:
		if err := s.store.Delete(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.audit("route.delete", map[string]any{"routeId": id})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.discoverer == nil {
		status := s.dockerStatus()
		writeJSON(w, http.StatusOK, map[string]any{"containers": []model.ContainerService{}, "status": status, "warning": status.Reason})
		return
	}
	containers, routeHints, err := s.discoverer.Discover(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	gatewayNetworks := gatewayContainerNetworks(containers)
	containers, routeHints = withoutGatewayContainer(containers, routeHints)
	for index := range containers {
		if port, err := bindPort(containers[index], 0); err == nil {
			policy := containerBindPolicy(containers[index], gatewayNetworks, port)
			containers[index].BindPolicy = &policy
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": containers, "routeHints": routeHints, "gatewayNetworks": gatewayNetworks, "status": s.dockerStatus()})
}

func (s *Server) handleBindContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.discoverer == nil {
		writeError(w, http.StatusConflict, "docker discovery is not active")
		return
	}
	var request bindContainerRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(request.ContainerID) == "" {
		writeError(w, http.StatusBadRequest, "containerId is required")
		return
	}

	containers, _, err := s.discoverer.Discover(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	container, ok := findContainer(containers, request.ContainerID)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}
	if gateway, found := gatewayContainer(containers); found && gateway.ID == container.ID {
		writeError(w, http.StatusBadRequest, "the gateway container cannot be bound as an upstream")
		return
	}
	port, err := bindPort(container, request.Port)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scheme, err := normalizeUpstreamScheme(request.Scheme)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	policy := containerBindPolicy(container, gatewayContainerNetworks(containers), port)
	if !policy.CanBind {
		writeError(w, http.StatusBadRequest, bindPolicyError(container, policy, port))
		return
	}
	host := strings.ToLower(strings.TrimSpace(request.Host))
	if host == "" {
		host = defaultBindHost(container)
	}
	exposure := normalizeExposure(request.Exposure)
	upstreamName := bindUpstreamName(container, policy.GatewayNetworks)
	route := model.RouteConfig{
		ID:        fmt.Sprintf("bind-%s-%d-%s", shortID(container.ID), port, slug(host)),
		Host:      host,
		Exposure:  exposure,
		Enabled:   true,
		HTTPS:     request.HTTPS,
		WebSocket: request.WebSocket,
		Source:    "explicit",
		Upstreams: []model.UpstreamTarget{{Name: upstreamName, URL: fmt.Sprintf("%s://%s:%d", scheme, upstreamName, port)}},
	}
	created, err := s.store.Add(route)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	s.audit("route.bind", map[string]any{"routeId": created.ID, "containerId": container.ID, "host": created.Host, "port": port, "scheme": scheme, "exposure": created.Exposure})
	s.markRoutingChangesPending()
	writeJSON(w, http.StatusCreated, map[string]any{"route": created, "pendingApply": true})
}

func (s *Server) dockerStatus() dockerStatus {
	cfg := s.configSnapshot()
	status := dockerStatus{
		Enabled:    cfg.Docker.Enabled,
		Active:     s.discoverer != nil,
		Profile:    string(cfg.Profile),
		SocketPath: cfg.Docker.SocketPath,
		Endpoint:   cfg.Docker.Endpoint,
	}
	switch {
	case !cfg.Docker.Enabled:
		status.Reason = "Docker discovery is disabled by configuration or GATEWAY_DOCKER_ENABLED"
		status.NextActions = []string{"Use explicit routes, or set GATEWAY_DOCKER_ENABLED=true for local workloads", "Mount /var/run/docker.sock read-only or provide a Docker socket proxy when discovery is enabled"}
	case s.discoverer == nil:
		status.Reason = "Docker discovery is configured but the discoverer was not initialized"
		status.NextActions = []string{"Check the gateway startup logs", "Verify the Docker socket path is reachable from the container"}
	default:
		status.Reason = "Docker discovery is active"
		status.NextActions = []string{"Add caddy.enable=true, caddy.host, and caddy.port labels to workload containers"}
	}
	return status
}

func findContainer(containers []model.ContainerService, id string) (model.ContainerService, bool) {
	id = strings.TrimSpace(id)
	for _, container := range containers {
		if container.ID == id || strings.HasPrefix(container.ID, id) || container.Name == id {
			return container, true
		}
	}
	return model.ContainerService{}, false
}

func withoutGatewayContainer(containers []model.ContainerService, routeHints []model.RouteConfig) ([]model.ContainerService, []model.RouteConfig) {
	gateway, found := gatewayContainer(containers)
	if !found {
		return containers, routeHints
	}
	visible := make([]model.ContainerService, 0, len(containers)-1)
	for _, container := range containers {
		if container.ID != gateway.ID {
			visible = append(visible, container)
		}
	}
	gatewayRouteID := "docker-" + shortID(gateway.ID)
	visibleHints := make([]model.RouteConfig, 0, len(routeHints))
	for _, route := range routeHints {
		if route.ID != gatewayRouteID {
			visibleHints = append(visibleHints, route)
		}
	}
	return visible, visibleHints
}

func normalizeUpstreamScheme(value string) (string, error) {
	scheme := strings.ToLower(strings.TrimSpace(value))
	if scheme == "" {
		return "http", nil
	}
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("upstream scheme must be http or https")
	}
	return scheme, nil
}

func bindPort(container model.ContainerService, requested int) (int, error) {
	if requested > 0 {
		for _, port := range container.Ports {
			if port.PrivatePort == requested && strings.EqualFold(port.Type, "tcp") {
				return requested, nil
			}
		}
		return 0, fmt.Errorf("container %s does not expose tcp port %d", container.Name, requested)
	}
	for _, port := range container.Ports {
		if port.PrivatePort > 0 && strings.EqualFold(port.Type, "tcp") {
			return port.PrivatePort, nil
		}
	}
	return 0, fmt.Errorf("container %s has no tcp ports to bind", container.Name)
}

func defaultBindHost(container model.ContainerService) string {
	if host := strings.TrimSpace(container.Labels["caddy.host"]); host != "" {
		return strings.ToLower(host)
	}
	return slug(container.Name) + ".localhost"
}

func bindUpstreamName(container model.ContainerService, gatewayNetworks []string) string {
	if sharedAddress := sharedNetworkAddress(container, gatewayNetworks); sharedAddress != "" {
		return sharedAddress
	}
	if len(container.NetworkAddresses) > 0 {
		return container.NetworkAddresses[0]
	}
	if name := strings.TrimSpace(container.Labels["com.docker.compose.service"]); name != "" {
		return name
	}
	return container.Name
}

func gatewayContainerNetworks(containers []model.ContainerService) []string {
	gateway, ok := gatewayContainer(containers)
	if !ok {
		return nil
	}
	output := make([]string, len(gateway.Networks))
	copy(output, gateway.Networks)
	return output
}

func gatewayContainer(containers []model.ContainerService) (model.ContainerService, bool) {
	hostname, err := os.Hostname()
	if err != nil {
		return model.ContainerService{}, false
	}
	return findContainer(containers, hostname)
}

func containerBindPolicy(container model.ContainerService, gatewayNetworks []string, port int) model.ContainerBindPolicy {
	policy := model.ContainerBindPolicy{
		CanBind:         true,
		Mode:            "unknown",
		GatewayNetworks: append([]string(nil), gatewayNetworks...),
	}
	if len(gatewayNetworks) == 0 {
		return policy
	}

	policy.SharedNetworks = sharedNetworks(container.Networks, gatewayNetworks)
	if len(policy.SharedNetworks) > 0 {
		policy.Mode = "shared-network"
		return policy
	}

	policy.CanBind = false
	if hasNetwork(container.Networks, "host") {
		policy.Mode = "host-network"
		policy.SuggestedUpstream = fmt.Sprintf("http://host.docker.internal:%d", port)
		return policy
	}
	if publishedPort := publishedTCPPort(container, port); publishedPort > 0 {
		policy.Mode = "published-port"
		policy.SuggestedUpstream = fmt.Sprintf("http://host.docker.internal:%d", publishedPort)
		return policy
	}
	if hasNetwork(container.Networks, "bridge") {
		policy.Mode = "bridge-unreachable"
		return policy
	}
	policy.Mode = "network-unreachable"
	return policy
}

func bindPolicyError(container model.ContainerService, policy model.ContainerBindPolicy, port int) string {
	joinedNetworks := strings.Join(policy.GatewayNetworks, ", ")
	switch policy.Mode {
	case "host-network":
		return fmt.Sprintf("container %s uses Docker host networking; add an explicit route to %s, or use http://127.0.0.1:%d only when the gateway itself runs in host mode", container.Name, policy.SuggestedUpstream, port)
	case "published-port":
		return fmt.Sprintf("container %s does not share a network with the gateway; add an explicit route to %s or attach the container to one of the gateway networks: %s", container.Name, policy.SuggestedUpstream, joinedNetworks)
	case "bridge-unreachable":
		return fmt.Sprintf("container %s is only on Docker bridge and does not share a network with the gateway; attach it to one of the gateway networks: %s", container.Name, joinedNetworks)
	case "network-unreachable":
		return fmt.Sprintf("container %s does not share a network with the gateway; attach it to one of the gateway networks: %s", container.Name, joinedNetworks)
	default:
		return fmt.Sprintf("container %s is not directly reachable from the gateway", container.Name)
	}
}

func sharedNetworks(containerNetworks []string, gatewayNetworks []string) []string {
	if len(containerNetworks) == 0 || len(gatewayNetworks) == 0 {
		return nil
	}
	gatewaySet := make(map[string]struct{}, len(gatewayNetworks))
	for _, network := range gatewayNetworks {
		gatewaySet[network] = struct{}{}
	}
	shared := make([]string, 0, len(containerNetworks))
	for _, network := range containerNetworks {
		if _, ok := gatewaySet[network]; ok {
			shared = append(shared, network)
		}
	}
	return shared
}

func sharedNetworkAddress(container model.ContainerService, gatewayNetworks []string) string {
	shared := sharedNetworks(container.Networks, gatewayNetworks)
	for _, network := range shared {
		for _, endpoint := range container.NetworkEndpoints {
			if endpoint.Name == network && strings.TrimSpace(endpoint.Address) != "" {
				return endpoint.Address
			}
		}
	}
	if len(shared) > 0 && !hasNetwork(shared, "bridge") {
		if name := strings.TrimSpace(container.Labels["com.docker.compose.service"]); name != "" {
			return name
		}
		return container.Name
	}
	return ""
}

func hasNetwork(networks []string, target string) bool {
	for _, network := range networks {
		if strings.EqualFold(network, target) {
			return true
		}
	}
	return false
}

func publishedTCPPort(container model.ContainerService, privatePort int) int {
	for _, port := range container.Ports {
		if !strings.EqualFold(port.Type, "tcp") || port.PublicPort == 0 {
			continue
		}
		if privatePort == 0 || port.PrivatePort == privatePort {
			return port.PublicPort
		}
	}
	return 0
}

func normalizeExposure(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "internal", "protected":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "public"
	}
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func slug(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(value) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		case char == '.', char == '-':
			if !lastDash {
				builder.WriteByte(byte(char))
				lastDash = char == '-'
			}
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	output := strings.Trim(builder.String(), "-.")
	if output == "" {
		return "service"
	}
	return output
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	result := s.reconciler.Sync(r.Context())
	if result.CaddyLoaded {
		if err := s.persistConfigurationImport(); err != nil {
			if result.Error != "" {
				result.Error += "; "
			}
			result.Error += "persist imported configuration: " + err.Error()
		} else {
			s.clearRoutingChangesPending()
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) markRoutingChangesPending() {
	if controller, ok := s.reconciler.(routingChangeController); ok {
		controller.MarkRoutingChangesPending()
	}
}

func (s *Server) clearRoutingChangesPending() {
	if controller, ok := s.reconciler.(routingChangeController); ok {
		controller.ClearRoutingChangesPending()
	}
}

func (s *Server) routingChangesPending() bool {
	controller, ok := s.reconciler.(routingChangeController)
	return ok && controller.RoutingChangesPending()
}

func (s *Server) handleCertificate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.configSnapshot()
		certificatePolicy := s.desiredCertificatePolicy(cfg.Gateway.Certificate)
		writeJSON(w, http.StatusOK, s.certificateResponse(certificateWithAzureDefaults(certificatePolicy, cfg.Azure)))
	case http.MethodPut:
		if s.rejectWhileConfigurationImportPending(w) {
			return
		}
		var cert model.CertificateConfig
		if err := json.NewDecoder(r.Body).Decode(&cert); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg := s.configSnapshot()
		current := cfg.Gateway.Certificate
		if cert.DNSChallenge.Provider == "azure" && strings.EqualFold(cert.DNSChallenge.Azure.Authentication, "appRegistration") && cert.DNSChallenge.Azure.ClientSecret == "" {
			cert.DNSChallenge.Azure.ClientSecret = current.DNSChallenge.Azure.ClientSecret
		}
		cert = certificateWithAzureDefaults(cert, cfg.Azure)
		cert = appconfig.NormalizeCertificateConfig(cert)
		if err := appconfig.ValidateCertificateConfig(cert); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if s.certificateStore == nil {
			writeError(w, http.StatusInternalServerError, "certificate persistence is unavailable")
			return
		}
		if err := s.certificateStore.Save(cert); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.updateCertificate(cert)
		s.audit("certificate.update", map[string]any{"issuer": cert.Issuer, "emailConfigured": cert.Email != "", "staging": cert.Staging, "customCA": cert.CADirectory != "", "subjects": len(cert.Subjects), "dnsProvider": cert.DNSChallenge.Provider})
		reconcile := s.reconciler.SyncWithoutPendingRoutingChanges(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"certificate": s.certificateResponse(cert), "reconcile": reconcile})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleCertificateRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.rejectWhileConfigurationImportPending(w) {
		return
	}
	cert := s.configSnapshot().Gateway.Certificate
	s.audit("certificate.refresh", map[string]any{"issuer": certificateIssuerName(cert), "emailConfigured": cert.Email != "", "staging": cert.Staging})
	reconcile := s.reconciler.SyncWithoutPendingRoutingChanges(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"certificate": s.certificateResponse(cert), "reconcile": reconcile})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	safe := s.configSnapshot()
	safe.Auth.AdminToken = ""
	safe.Auth.AdminTokens = nil
	safe.Auth.ProtectedRoutes.AdditionalHeaderValue = ""
	safe.Gateway.Certificate.DNSChallenge.Azure.ClientSecret = ""
	writeJSON(w, http.StatusOK, safe)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.auditLog == nil {
		writeJSON(w, http.StatusOK, map[string]any{"events": []audit.Event{}})
		return
	}
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	events, err := s.auditLog.ReadLast(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := 200
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	entries := []logs.Entry{}
	if s.runtimeLogs != nil {
		entries = s.runtimeLogs.ReadLast(limit)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) audit(event string, fields map[string]any) {
	if s.auditLog == nil {
		return
	}
	if err := s.auditLog.Record(context.Background(), event, fields); err != nil && s.logger != nil {
		s.logger.Warn("write audit event failed", "error", err)
	}
}

func (s *Server) updateCertificate(cert model.CertificateConfig) {
	s.mu.Lock()
	s.cfg.Gateway.Certificate = cert
	s.mu.Unlock()
	if updater, ok := s.reconciler.(certificateUpdater); ok {
		updater.UpdateCertificate(cert)
	}
}

func (s *Server) configSnapshot() model.AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func certificateStatus(cfg model.AppConfig) map[string]any {
	return certificateStatusWithPolicy(cfg, cfg.Gateway.Certificate)
}

func certificateStatusWithPolicy(cfg model.AppConfig, cert model.CertificateConfig) map[string]any {
	return map[string]any{
		"issuer":             certificateIssuerName(cert),
		"emailConfigured":    cert.Email != "",
		"staging":            cert.Staging,
		"caDirectory":        cert.CADirectory,
		"subjects":           cert.Subjects,
		"renewalWindowRatio": cert.RenewalWindowRatio,
		"dnsProvider":        cert.DNSChallenge.Provider,
		"persisted":          cfg.Gateway.CertificateFile != "",
	}
}

func certificateConfigResponse(cert model.CertificateConfig) map[string]any {
	cert = appconfig.NormalizeCertificateConfig(cert)
	azure := cert.DNSChallenge.Azure
	return map[string]any{
		"issuer":             cert.Issuer,
		"email":              cert.Email,
		"emailConfigured":    cert.Email != "",
		"staging":            cert.Staging,
		"caDirectory":        cert.CADirectory,
		"subjects":           cert.Subjects,
		"renewalWindowRatio": cert.RenewalWindowRatio,
		"dnsChallenge": map[string]any{
			"provider": cert.DNSChallenge.Provider,
			"azure": map[string]any{
				"subscriptionId":         azure.SubscriptionID,
				"resourceGroup":          azure.ResourceGroup,
				"authentication":         azure.Authentication,
				"tenantId":               azure.TenantID,
				"clientId":               azure.ClientID,
				"clientSecretConfigured": azure.ClientSecret != "",
			},
		},
		"runtimeOnly": false,
		"persisted":   true,
	}
}

func (s *Server) certificateResponse(cert model.CertificateConfig) map[string]any {
	response := certificateConfigResponse(cert)
	runtime := map[string]any{
		"available":    s.certificateInspector != nil,
		"certificates": []certificate.Status{},
	}
	if s.certificateInspector != nil {
		snapshot, err := s.certificateInspector.Inspect(cert.RenewalWindowRatio)
		if err != nil {
			runtime["available"] = false
			runtime["error"] = err.Error()
		} else {
			runtime["storageDirectory"] = snapshot.StorageDirectory
			runtime["scannedAt"] = snapshot.ScannedAt
			runtime["certificates"] = snapshot.Certificates
			runtime["warnings"] = snapshot.Warnings
		}
	}
	response["runtime"] = runtime
	return response
}

func certificateWithAzureDefaults(cert model.CertificateConfig, azure model.AzureConfig) model.CertificateConfig {
	if !azure.Enabled || !azure.ManageDNS {
		return cert
	}
	return appconfig.ApplyAzureCertificateDefaults(cert, azure)
}

func certificateIssuerName(cert model.CertificateConfig) string {
	return appconfig.NormalizeCertificateConfig(cert).Issuer
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
