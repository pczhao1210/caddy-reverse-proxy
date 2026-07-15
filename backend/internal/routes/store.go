package routes

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
)

type fileFormat struct {
	Routes []model.RouteConfig `json:"routes"`
}

type Store struct {
	path   string
	mu     sync.RWMutex
	routes []model.RouteConfig
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
	for index := range payload.Routes {
		normalize(&payload.Routes[index])
	}
	s.routes = payload.Routes
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
}

func (s *Store) Add(route model.RouteConfig) (model.RouteConfig, error) {
	if err := validate(route); err != nil {
		return route, err
	}
	normalize(&route)
	s.mu.Lock()
	defer s.mu.Unlock()
	if route.ID == "" {
		route.ID = fmt.Sprintf("route-%d", time.Now().UnixNano())
	}
	for _, existing := range s.routes {
		if existing.ID == route.ID {
			return route, fmt.Errorf("route id %q already exists", route.ID)
		}
		if existing.Host == route.Host && existing.PathPrefix == route.PathPrefix {
			return route, fmt.Errorf("route for host %q already exists", route.Host)
		}
	}
	route.Source = sourceOr(route.Source, "explicit")
	route.LastUpdated = time.Now().UTC()
	s.routes = append(s.routes, route)
	if err := s.saveLocked(); err != nil {
		s.routes = s.routes[:len(s.routes)-1]
		return route, err
	}
	return route, nil
}

func (s *Store) Replace(route model.RouteConfig) (model.RouteConfig, error) {
	if route.ID == "" {
		return route, fmt.Errorf("route id is required")
	}
	if err := validate(route); err != nil {
		return route, err
	}
	normalize(&route)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.routes {
		if existing.ID != route.ID && existing.Host == route.Host && existing.PathPrefix == route.PathPrefix {
			return route, fmt.Errorf("route for host %q and path %q already exists", route.Host, route.PathPrefix)
		}
	}
	for index := range s.routes {
		if s.routes[index].ID == route.ID {
			previous := s.routes[index]
			route.LastUpdated = time.Now().UTC()
			s.routes[index] = route
			if err := s.saveLocked(); err != nil {
				s.routes[index] = previous
				return route, err
			}
			return route, nil
		}
	}
	return route, fmt.Errorf("route id %q not found", route.ID)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.routes {
		if s.routes[index].ID == id {
			previous := s.routes[index]
			s.routes = append(s.routes[:index], s.routes[index+1:]...)
			if err := s.saveLocked(); err != nil {
				s.routes = append(s.routes[:index], append([]model.RouteConfig{previous}, s.routes[index:]...)...)
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("route id %q not found", id)
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fileFormat{Routes: s.routes}, "", "  ")
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

func sourceOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
