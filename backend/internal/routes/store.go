package routes

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aidockerfarm/gateway/internal/model"
)

type fileFormat struct {
	Version      int                 `json:"version,omitempty"`
	Listeners    []model.Listener    `json:"listeners,omitempty"`
	BackendPools []model.BackendPool `json:"backendPools,omitempty"`
	RoutingRules []model.RoutingRule `json:"routingRules,omitempty"`
	Routes       []model.RouteConfig `json:"routes,omitempty"`
}

type Store struct {
	path         string
	mu           sync.RWMutex
	listeners    []model.Listener
	backendPools []model.BackendPool
	routingRules []model.RoutingRule
	routes       []model.RouteConfig
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var payload fileFormat
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.Version > 2 {
		return fmt.Errorf("routes file version %d is not supported", payload.Version)
	}
	snapshot := s.snapshotLocked()
	s.listeners = nil
	s.backendPools = nil
	s.routingRules = nil
	s.routes = nil
	migrated := false
	if payload.Version >= 2 || len(payload.Listeners) > 0 || len(payload.BackendPools) > 0 || len(payload.RoutingRules) > 0 {
		s.listeners = payload.Listeners
		s.backendPools = payload.BackendPools
		s.routingRules = payload.RoutingRules
		if err := s.normalizeResourcesLocked(); err != nil {
			s.restoreLocked(snapshot)
			return err
		}
	} else {
		migrated = true
		for _, route := range payload.Routes {
			if _, err := s.migrateRouteLocked(route); err != nil {
				s.restoreLocked(snapshot)
				return fmt.Errorf("migrate route %q: %w", route.ID, err)
			}
		}
	}
	if err := s.rebuildRoutesLocked(); err != nil {
		s.restoreLocked(snapshot)
		return err
	}
	if migrated {
		if err := s.saveLocked(); err != nil {
			s.restoreLocked(snapshot)
			return fmt.Errorf("save migrated routes: %w", err)
		}
	}
	return nil
}

func (s *Store) List() []model.RouteConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	output := make([]model.RouteConfig, len(s.routes))
	copy(output, s.routes)
	return output
}

func (s *Store) SetRuntimeStatus(statuses []model.RouteHealthStatus) {
	if len(statuses) == 0 {
		return
	}
	byID := make(map[string]model.RouteHealthStatus, len(statuses))
	for _, status := range statuses {
		byID[status.RouteID] = status
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.routes {
		status, ok := byID[s.routes[index].ID]
		if !ok {
			continue
		}
		if status.Healthy {
			s.routes[index].LastError = ""
		} else {
			s.routes[index].LastError = status.Error
		}
	}
	for index := range s.routingRules {
		status, ok := byID[s.routingRules[index].ID]
		if !ok {
			continue
		}
		if status.Healthy {
			s.routingRules[index].LastError = ""
		} else {
			s.routingRules[index].LastError = status.Error
		}
	}
}

func (s *Store) Add(route model.RouteConfig) (model.RouteConfig, error) {
	return s.addCompatibilityRoute(route)
}

func (s *Store) Replace(route model.RouteConfig) (model.RouteConfig, error) {
	return s.replaceCompatibilityRoute(route)
}

func (s *Store) Delete(id string) error {
	return s.deleteCompatibilityRoute(id)
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fileFormat{
		Version:      2,
		Listeners:    s.listeners,
		BackendPools: s.backendPools,
		RoutingRules: s.routingRules,
	}, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}

func validate(route model.RouteConfig) error {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(route.Host)), ".")
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if err := validateRouteHost(host); err != nil {
		return err
	}
	if len(route.Upstreams) == 0 {
		return fmt.Errorf("at least one upstream is required")
	}
	pathPrefix := strings.TrimSpace(route.PathPrefix)
	if pathPrefix != "" {
		if !strings.HasPrefix(pathPrefix, "/") {
			return fmt.Errorf("pathPrefix must start with /")
		}
		if strings.Contains(pathPrefix, "*") {
			return fmt.Errorf("pathPrefix must not contain wildcards")
		}
	}
	for _, upstream := range route.Upstreams {
		rawURL := strings.TrimSpace(upstream.URL)
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Host == "" || parsed.Scheme == "" {
			return fmt.Errorf("upstream url %q must include scheme and host", upstream.URL)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
		default:
			return fmt.Errorf("upstream url %q must use http or https", upstream.URL)
		}
	}
	if err := validateRouteSecurity(route.Security); err != nil {
		return err
	}
	return nil
}

func validateRouteSecurity(security model.RouteSecurityConfig) error {
	if security.MaxRequestBodyBytes < 0 {
		return fmt.Errorf("security.maxRequestBodyBytes must not be negative")
	}
	for _, method := range security.AdditionalDeniedMethods {
		if !validHTTPToken(strings.ToUpper(strings.TrimSpace(method))) {
			return fmt.Errorf("security.additionalDeniedMethods contains invalid HTTP method %q", method)
		}
	}
	for _, prefix := range security.AdditionalDeniedPathPrefixes {
		prefix = strings.TrimSpace(prefix)
		if !strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "*") {
			return fmt.Errorf("security.additionalDeniedPathPrefixes entry %q must start with / and must not contain wildcards", prefix)
		}
	}
	if err := validateIPRanges("security.allowedCidrs", security.AllowedCIDRs); err != nil {
		return err
	}
	return validateIPRanges("security.blockedCidrs", security.BlockedCIDRs)
}

func validHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func validateIPRanges(field string, ranges []string) error {
	for _, source := range ranges {
		source = strings.TrimSpace(source)
		if net.ParseIP(source) == nil {
			if _, _, err := net.ParseCIDR(source); err != nil {
				return fmt.Errorf("%s contains invalid IP or CIDR %q", field, source)
			}
		}
	}
	return nil
}

func validateRouteHost(host string) error {
	name := host
	if strings.HasPrefix(host, "*.") {
		name = strings.TrimPrefix(host, "*.")
	}
	if strings.Contains(name, "*") || strings.ContainsAny(name, "/:") || len(name) > 253 {
		return fmt.Errorf("host %q must be a DNS name with at most one left-most wildcard", host)
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return fmt.Errorf("host %q must be a fully qualified DNS name", host)
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("host %q is not a valid DNS name", host)
		}
		for _, character := range label {
			if character < 'a' || character > 'z' {
				if character < '0' || character > '9' {
					if character != '-' {
						return fmt.Errorf("host %q is not a valid DNS name", host)
					}
				}
			}
		}
	}
	return nil
}

func normalize(route *model.RouteConfig) {
	route.Host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(route.Host)), ".")
	route.PathPrefix = strings.TrimSpace(route.PathPrefix)
	for index := range route.Upstreams {
		route.Upstreams[index].URL = strings.TrimSpace(route.Upstreams[index].URL)
	}
	route.Security.AdditionalDeniedMethods = normalizeMethods(route.Security.AdditionalDeniedMethods)
	route.Security.AdditionalDeniedPathPrefixes = normalizePathPrefixes(route.Security.AdditionalDeniedPathPrefixes)
	route.Security.AllowedCIDRs = normalizeStrings(route.Security.AllowedCIDRs)
	route.Security.BlockedCIDRs = normalizeStrings(route.Security.BlockedCIDRs)
	if route.PathPrefix != "/" {
		route.PathPrefix = strings.TrimRight(route.PathPrefix, "/")
	}
	route.Exposure = strings.ToLower(strings.TrimSpace(route.Exposure))
	if route.Source == "" {
		route.Source = "explicit"
	}
	if route.Exposure == "" {
		if route.Public || route.Source == "explicit" {
			route.Exposure = "public"
		} else {
			route.Exposure = "internal"
		}
	}
	switch route.Exposure {
	case "internal":
		route.Public = false
		route.Protected = false
	case "protected":
		route.Public = true
		route.Protected = true
	default:
		route.Exposure = "public"
		route.Public = true
		route.Protected = false
	}
}

func normalizeMethods(methods []string) []string {
	output := normalizeStrings(methods)
	for index := range output {
		output[index] = strings.ToUpper(output[index])
	}
	return uniqueStrings(output)
}

func normalizePathPrefixes(prefixes []string) []string {
	output := normalizeStrings(prefixes)
	for index := range output {
		if output[index] != "/" {
			output[index] = strings.TrimRight(output[index], "/")
		}
	}
	return uniqueStrings(output)
}

func normalizeStrings(values []string) []string {
	output := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			output = append(output, value)
		}
	}
	return uniqueStrings(output)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	output := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		output = append(output, value)
	}
	return output
}

func sourceOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
