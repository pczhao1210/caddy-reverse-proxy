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
	appconfig "github.com/aidockerfarm/gateway/internal/config"
	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
	uiassets "github.com/aidockerfarm/gateway/internal/ui"
)

type Discoverer interface {
	Discover(context.Context) ([]model.ContainerService, []model.RouteConfig, error)
}

type Reconciler interface {
	Sync(context.Context) model.ReconcileResult
	Last() model.ReconcileResult
}

type RuntimeStatus interface {
	Ready() bool
	LastError() error
}

type certificateUpdater interface {
	UpdateCertificate(model.CertificateConfig)
}

type CertificateStore interface {
	Save(model.CertificateConfig) error
}

type Options struct {
	Config           model.AppConfig
	Store            *routes.Store
	Discoverer       Discoverer
	Reconciler       Reconciler
	Runtime          RuntimeStatus
	AuditLog         AuditLog
	CertificateStore CertificateStore
	Logger           *slog.Logger
}

type Server struct {
	mu               sync.RWMutex
	cfg              model.AppConfig
	store            *routes.Store
	discoverer       Discoverer
	reconciler       Reconciler
	runtime          RuntimeStatus
	auditLog         AuditLog
	certificateStore CertificateStore
	logger           *slog.Logger
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
	Exposure    string `json:"exposure"`
	HTTPS       bool   `json:"https"`
	WebSocket   bool   `json:"websocket"`
}

func NewServer(options Options) *Server {
	return &Server{cfg: options.Config, store: options.Store, discoverer: options.Discoverer, reconciler: options.Reconciler, runtime: options.Runtime, auditLog: options.AuditLog, certificateStore: options.CertificateStore, logger: options.Logger}
}

func (s *Server) Handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("/healthz", s.handleReady)
	root.HandleFunc("/livez", s.handleLive)
	root.HandleFunc("/readyz", s.handleReady)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/status", s.handleStatus)
	apiMux.HandleFunc("/api/routes", s.handleRoutes)
	apiMux.HandleFunc("/api/routes/", s.handleRouteByID)
	apiMux.HandleFunc("/api/discovery/bind", s.handleBindContainer)
	apiMux.HandleFunc("/api/discovery/containers", s.handleContainers)
	apiMux.HandleFunc("/api/reconcile", s.handleReconcile)
	apiMux.HandleFunc("/api/certificate", s.handleCertificate)
	apiMux.HandleFunc("/api/certificate/refresh", s.handleCertificateRefresh)
	apiMux.HandleFunc("/api/config", s.handleConfig)
	apiMux.HandleFunc("/api/audit", s.handleAudit)
	root.Handle("/api/", auth.Middleware(s.configSnapshot().Auth, apiMux))

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
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":       cfg.Profile,
		"routes":        s.store.List(),
		"lastReconcile": s.reconciler.Last(),
		"azure":         azure.StatusForConfig(cfg),
		"docker":        s.dockerStatus(),
		"certificate":   certificateStatus(cfg),
		"security":      cfg.Security,
		"health":        cfg.Health,
		"audit":         map[string]any{"enabled": cfg.Audit.Enabled, "file": cfg.Audit.File},
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
		writeJSON(w, http.StatusCreated, map[string]any{"route": created, "reconcile": s.reconciler.Sync(r.Context())})
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
		writeJSON(w, http.StatusOK, map[string]any{"route": updated, "reconcile": s.reconciler.Sync(r.Context())})
	case http.MethodDelete:
		if err := s.store.Delete(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.audit("route.delete", map[string]any{"routeId": id})
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id, "reconcile": s.reconciler.Sync(r.Context())})
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
	port, err := bindPort(container, request.Port)
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
		Upstreams: []model.UpstreamTarget{{Name: upstreamName, URL: fmt.Sprintf("http://%s:%d", upstreamName, port)}},
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
	s.audit("route.bind", map[string]any{"routeId": created.ID, "containerId": container.ID, "host": created.Host, "port": port, "exposure": created.Exposure})
	writeJSON(w, http.StatusCreated, map[string]any{"route": created, "reconcile": s.reconciler.Sync(r.Context())})
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
	hostname, err := os.Hostname()
	if err != nil {
		return nil
	}
	gateway, ok := findContainer(containers, hostname)
	if !ok {
		return nil
	}
	output := make([]string, len(gateway.Networks))
	copy(output, gateway.Networks)
	return output
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
	writeJSON(w, http.StatusOK, s.reconciler.Sync(r.Context()))
}

func (s *Server) handleCertificate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, certificateConfigResponse(s.configSnapshot().Gateway.Certificate))
	case http.MethodPut:
		var cert model.CertificateConfig
		if err := json.NewDecoder(r.Body).Decode(&cert); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		current := s.configSnapshot().Gateway.Certificate
		if cert.DNSChallenge.Provider == "azure" && strings.EqualFold(cert.DNSChallenge.Azure.Authentication, "appRegistration") && cert.DNSChallenge.Azure.ClientSecret == "" {
			cert.DNSChallenge.Azure.ClientSecret = current.DNSChallenge.Azure.ClientSecret
		}
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
		writeJSON(w, http.StatusOK, map[string]any{"certificate": certificateConfigResponse(cert), "reconcile": s.reconciler.Sync(r.Context())})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleCertificateRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	cert := s.configSnapshot().Gateway.Certificate
	s.audit("certificate.refresh", map[string]any{"issuer": certificateIssuerName(cert), "emailConfigured": cert.Email != "", "staging": cert.Staging})
	writeJSON(w, http.StatusOK, map[string]any{"certificate": certificateConfigResponse(cert), "reconcile": s.reconciler.Sync(r.Context())})
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
	cert := cfg.Gateway.Certificate
	return map[string]any{
		"issuer":          certificateIssuerName(cert),
		"emailConfigured": cert.Email != "",
		"staging":         cert.Staging,
		"caDirectory":     cert.CADirectory,
		"subjects":        cert.Subjects,
		"dnsProvider":     cert.DNSChallenge.Provider,
		"persisted":       cfg.Gateway.CertificateFile != "",
	}
}

func certificateConfigResponse(cert model.CertificateConfig) map[string]any {
	cert = appconfig.NormalizeCertificateConfig(cert)
	azure := cert.DNSChallenge.Azure
	return map[string]any{
		"issuer":          cert.Issuer,
		"email":           cert.Email,
		"emailConfigured": cert.Email != "",
		"staging":         cert.Staging,
		"caDirectory":     cert.CADirectory,
		"subjects":        cert.Subjects,
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
