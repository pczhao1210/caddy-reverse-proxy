package routes

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
)

type storeSnapshot struct {
	listeners    []model.Listener
	backendPools []model.BackendPool
	routingRules []model.RoutingRule
	routes       []model.RouteConfig
}

func (s *Store) Listeners() []model.Listener {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Listener(nil), s.listeners...)
}

func (s *Store) BackendPools() []model.BackendPool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneBackendPools(s.backendPools)
}

func (s *Store) RoutingRules() []model.RoutingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRoutingRules(s.routingRules)
}

func (s *Store) AddListener(listener model.Listener) (model.Listener, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshotLocked()
	created, err := s.addListenerLocked(listener)
	if err != nil {
		return listener, err
	}
	if err := s.commitLocked(snapshot); err != nil {
		return listener, err
	}
	return created, nil
}

func (s *Store) ReplaceListener(listener model.Listener) (model.Listener, error) {
	if listener.ID == "" {
		return listener, fmt.Errorf("listener id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshotLocked()
	normalizeListener(&listener)
	if err := validateListener(listener); err != nil {
		return listener, err
	}
	for _, existing := range s.listeners {
		if existing.ID != listener.ID && sameListener(existing, listener) {
			return listener, fmt.Errorf("listener for %s already exists", listenerURL(listener))
		}
	}
	found := false
	listener.LastUpdated = time.Now().UTC()
	for index := range s.listeners {
		if s.listeners[index].ID == listener.ID {
			s.listeners[index] = listener
			found = true
			break
		}
	}
	if !found {
		return listener, fmt.Errorf("listener id %q not found", listener.ID)
	}
	if err := s.commitLocked(snapshot); err != nil {
		return listener, err
	}
	return listener, nil
}

func (s *Store) DeleteListener(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rule := range s.routingRules {
		if rule.ListenerID == id {
			return fmt.Errorf("listener %q is used by routing rule %q", id, rule.ID)
		}
	}
	snapshot := s.snapshotLocked()
	if !deleteListenerByID(&s.listeners, id) {
		return fmt.Errorf("listener id %q not found", id)
	}
	return s.commitLocked(snapshot)
}

func (s *Store) AddBackendPool(pool model.BackendPool) (model.BackendPool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshotLocked()
	created, err := s.addBackendPoolLocked(pool)
	if err != nil {
		return pool, err
	}
	if err := s.commitLocked(snapshot); err != nil {
		return pool, err
	}
	return created, nil
}

func (s *Store) ReplaceBackendPool(pool model.BackendPool) (model.BackendPool, error) {
	if pool.ID == "" {
		return pool, fmt.Errorf("backend pool id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshotLocked()
	normalizeBackendPool(&pool)
	if err := validateBackendPool(pool); err != nil {
		return pool, err
	}
	for _, existing := range s.backendPools {
		if existing.ID != pool.ID && strings.EqualFold(existing.Name, pool.Name) {
			return pool, fmt.Errorf("backend pool name %q already exists", pool.Name)
		}
	}
	found := false
	pool.LastUpdated = time.Now().UTC()
	for index := range s.backendPools {
		if s.backendPools[index].ID == pool.ID {
			s.backendPools[index] = pool
			found = true
			break
		}
	}
	if !found {
		return pool, fmt.Errorf("backend pool id %q not found", pool.ID)
	}
	if err := s.commitLocked(snapshot); err != nil {
		return pool, err
	}
	return pool, nil
}

func (s *Store) DeleteBackendPool(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rule := range s.routingRules {
		if rule.BackendPoolID == id {
			return fmt.Errorf("backend pool %q is used by routing rule %q", id, rule.ID)
		}
	}
	snapshot := s.snapshotLocked()
	if !deleteBackendPoolByID(&s.backendPools, id) {
		return fmt.Errorf("backend pool id %q not found", id)
	}
	return s.commitLocked(snapshot)
}

func (s *Store) AddRoutingRule(rule model.RoutingRule) (model.RoutingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshotLocked()
	created, err := s.addRoutingRuleLocked(rule)
	if err != nil {
		return rule, err
	}
	if err := s.commitLocked(snapshot); err != nil {
		return rule, err
	}
	return created, nil
}

func (s *Store) ReplaceRoutingRule(rule model.RoutingRule) (model.RoutingRule, error) {
	if rule.ID == "" {
		return rule, fmt.Errorf("routing rule id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshotLocked()
	normalizeRoutingRule(&rule)
	if err := s.validateRoutingRuleLocked(rule, rule.ID); err != nil {
		return rule, err
	}
	found := false
	rule.LastUpdated = time.Now().UTC()
	for index := range s.routingRules {
		if s.routingRules[index].ID == rule.ID {
			rule.LastError = s.routingRules[index].LastError
			s.routingRules[index] = rule
			found = true
			break
		}
	}
	if !found {
		return rule, fmt.Errorf("routing rule id %q not found", rule.ID)
	}
	if err := s.commitLocked(snapshot); err != nil {
		return rule, err
	}
	return rule, nil
}

func (s *Store) DeleteRoutingRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshotLocked()
	if !deleteRoutingRuleByID(&s.routingRules, id) {
		return fmt.Errorf("routing rule id %q not found", id)
	}
	return s.commitLocked(snapshot)
}

func (s *Store) addCompatibilityRoute(route model.RouteConfig) (model.RouteConfig, error) {
	if err := validate(route); err != nil {
		return route, err
	}
	normalize(&route)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.routes {
		if route.ID != "" && existing.ID == route.ID {
			return route, fmt.Errorf("route id %q already exists", route.ID)
		}
		if existing.Host == route.Host && existing.PathPrefix == route.PathPrefix {
			return route, fmt.Errorf("route for host %q already exists", route.Host)
		}
	}
	snapshot := s.snapshotLocked()
	created, err := s.migrateRouteLocked(route)
	if err != nil {
		s.restoreLocked(snapshot)
		return route, err
	}
	if err := s.commitLocked(snapshot); err != nil {
		return route, err
	}
	return created, nil
}

func (s *Store) replaceCompatibilityRoute(route model.RouteConfig) (model.RouteConfig, error) {
	if route.ID == "" {
		return route, fmt.Errorf("route id is required")
	}
	if err := validate(route); err != nil {
		return route, err
	}
	normalize(&route)
	s.mu.Lock()
	defer s.mu.Unlock()
	if findRoutingRule(s.routingRules, route.ID) == nil {
		return route, fmt.Errorf("route id %q not found", route.ID)
	}
	for _, existing := range s.routes {
		if existing.ID != route.ID && existing.Host == route.Host && existing.PathPrefix == route.PathPrefix {
			return route, fmt.Errorf("route for host %q and path %q already exists", route.Host, route.PathPrefix)
		}
	}
	snapshot := s.snapshotLocked()
	oldRule := *findRoutingRule(s.routingRules, route.ID)
	deleteRoutingRuleByID(&s.routingRules, route.ID)
	s.pruneCompatibilityResourcesLocked(oldRule)
	created, err := s.migrateRouteLocked(route)
	if err != nil {
		s.restoreLocked(snapshot)
		return route, err
	}
	if err := s.commitLocked(snapshot); err != nil {
		return route, err
	}
	return created, nil
}

func (s *Store) deleteCompatibilityRoute(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule := findRoutingRule(s.routingRules, id)
	if rule == nil {
		return fmt.Errorf("route id %q not found", id)
	}
	snapshot := s.snapshotLocked()
	removed := *rule
	deleteRoutingRuleByID(&s.routingRules, id)
	s.pruneCompatibilityResourcesLocked(removed)
	return s.commitLocked(snapshot)
}

func (s *Store) migrateRouteLocked(route model.RouteConfig) (model.RouteConfig, error) {
	if err := validate(route); err != nil {
		return route, err
	}
	normalize(&route)
	if route.ID == "" {
		route.ID = nextID("route")
	}
	protocol := strings.ToLower(strings.TrimSpace(route.ListenerProtocol))
	if protocol == "" {
		if route.HTTPS {
			protocol = "https"
		} else {
			protocol = "http"
		}
	}
	listenerPort := route.ListenerPort
	if listenerPort == 0 {
		listenerPort = defaultPort(protocol)
	}
	listener := model.Listener{
		Name:     route.Host,
		Hostname: route.Host,
		Port:     listenerPort,
		Protocol: protocol,
		Source:   "compatibility",
	}
	if existing := s.findMatchingListenerLocked(listener); existing != nil {
		listener = *existing
	} else {
		var err error
		listener, err = s.addListenerLocked(listener)
		if err != nil {
			return route, err
		}
	}

	pool := model.BackendPool{
		ID:     uniqueID("pool-"+resourceIDPart(route.ID), backendPoolIDs(s.backendPools)),
		Name:   route.ID + " backends",
		Source: "compatibility",
	}
	backendProtocol := ""
	backendPort := 0
	commonHealthPath := ""
	for index, upstream := range route.Upstreams {
		parsed, err := url.Parse(strings.TrimSpace(upstream.URL))
		if err != nil || parsed.Hostname() == "" {
			return route, fmt.Errorf("upstream url %q is invalid", upstream.URL)
		}
		scheme := strings.ToLower(parsed.Scheme)
		if backendProtocol == "" {
			backendProtocol = scheme
		} else if backendProtocol != scheme {
			return route, fmt.Errorf("all upstreams must use the same http or https scheme")
		}
		port := defaultPort(scheme)
		if parsed.Port() != "" {
			parsedPort, err := strconv.Atoi(parsed.Port())
			if err != nil {
				return route, fmt.Errorf("upstream url %q has an invalid port", upstream.URL)
			}
			port = parsedPort
		}
		if backendPort == 0 {
			backendPort = port
		}
		target := model.BackendTarget{Name: upstream.Name, Address: parsed.Hostname(), HealthPath: upstream.HealthPath}
		if port != backendPort {
			target.Port = port
		}
		pool.Targets = append(pool.Targets, target)
		if index == 0 {
			commonHealthPath = upstream.HealthPath
		} else if commonHealthPath != upstream.HealthPath {
			commonHealthPath = ""
		}
	}
	if commonHealthPath != "" {
		for index := range pool.Targets {
			pool.Targets[index].HealthPath = ""
		}
	}
	pool, err := s.addBackendPoolLocked(pool)
	if err != nil {
		return route, err
	}
	rule := model.RoutingRule{
		ID:              route.ID,
		Name:            route.ID,
		ListenerID:      listener.ID,
		BackendPoolID:   pool.ID,
		BackendPort:     backendPort,
		BackendProtocol: backendProtocol,
		PathPrefix:      route.PathPrefix,
		HealthPath:      commonHealthPath,
		Exposure:        route.Exposure,
		Enabled:         route.Enabled,
		WebSocket:       route.WebSocket,
		Headers:         route.Headers,
		Security:        route.Security,
		Source:          sourceOr(route.Source, "explicit"),
		LastError:       route.LastError,
	}
	if _, err := s.addRoutingRuleLocked(rule); err != nil {
		return route, err
	}
	if err := s.rebuildRoutesLocked(); err != nil {
		return route, err
	}
	for _, compiled := range s.routes {
		if compiled.ID == route.ID {
			return compiled, nil
		}
	}
	return route, fmt.Errorf("route %q was not compiled", route.ID)
}

func (s *Store) addListenerLocked(listener model.Listener) (model.Listener, error) {
	normalizeListener(&listener)
	if listener.ID == "" {
		listener.ID = nextID("listener")
	}
	if listener.Name == "" {
		listener.Name = listener.Hostname
	}
	if listener.Source == "" {
		listener.Source = "explicit"
	}
	if err := validateListener(listener); err != nil {
		return listener, err
	}
	for _, existing := range s.listeners {
		if existing.ID == listener.ID {
			return listener, fmt.Errorf("listener id %q already exists", listener.ID)
		}
		if sameListener(existing, listener) {
			return listener, fmt.Errorf("listener for %s already exists", listenerURL(listener))
		}
	}
	listener.LastUpdated = time.Now().UTC()
	s.listeners = append(s.listeners, listener)
	return listener, nil
}

func (s *Store) addBackendPoolLocked(pool model.BackendPool) (model.BackendPool, error) {
	normalizeBackendPool(&pool)
	if pool.ID == "" {
		pool.ID = nextID("pool")
	}
	if pool.Source == "" {
		pool.Source = "explicit"
	}
	if err := validateBackendPool(pool); err != nil {
		return pool, err
	}
	for _, existing := range s.backendPools {
		if existing.ID == pool.ID {
			return pool, fmt.Errorf("backend pool id %q already exists", pool.ID)
		}
		if strings.EqualFold(existing.Name, pool.Name) {
			return pool, fmt.Errorf("backend pool name %q already exists", pool.Name)
		}
	}
	pool.LastUpdated = time.Now().UTC()
	s.backendPools = append(s.backendPools, pool)
	return pool, nil
}

func (s *Store) addRoutingRuleLocked(rule model.RoutingRule) (model.RoutingRule, error) {
	normalizeRoutingRule(&rule)
	if rule.ID == "" {
		rule.ID = nextID("rule")
	}
	if rule.Name == "" {
		rule.Name = rule.ID
	}
	if rule.Source == "" {
		rule.Source = "explicit"
	}
	if err := s.validateRoutingRuleLocked(rule, ""); err != nil {
		return rule, err
	}
	for _, existing := range s.routingRules {
		if existing.ID == rule.ID {
			return rule, fmt.Errorf("routing rule id %q already exists", rule.ID)
		}
	}
	rule.LastUpdated = time.Now().UTC()
	s.routingRules = append(s.routingRules, rule)
	return rule, nil
}

func (s *Store) normalizeResourcesLocked() error {
	listenerIDs := make(map[string]struct{}, len(s.listeners))
	listenerEndpoints := make(map[string]struct{}, len(s.listeners))
	for index := range s.listeners {
		normalizeListener(&s.listeners[index])
		if s.listeners[index].ID == "" {
			return fmt.Errorf("listener id is required")
		}
		if _, exists := listenerIDs[s.listeners[index].ID]; exists {
			return fmt.Errorf("listener id %q already exists", s.listeners[index].ID)
		}
		listenerIDs[s.listeners[index].ID] = struct{}{}
		endpoint := listenerURL(s.listeners[index])
		if _, exists := listenerEndpoints[endpoint]; exists {
			return fmt.Errorf("listener for %s already exists", endpoint)
		}
		listenerEndpoints[endpoint] = struct{}{}
		if err := validateListener(s.listeners[index]); err != nil {
			return fmt.Errorf("listener %q: %w", s.listeners[index].ID, err)
		}
	}
	poolIDs := make(map[string]struct{}, len(s.backendPools))
	poolNames := make(map[string]struct{}, len(s.backendPools))
	for index := range s.backendPools {
		normalizeBackendPool(&s.backendPools[index])
		if s.backendPools[index].ID == "" {
			return fmt.Errorf("backend pool id is required")
		}
		if _, exists := poolIDs[s.backendPools[index].ID]; exists {
			return fmt.Errorf("backend pool id %q already exists", s.backendPools[index].ID)
		}
		poolIDs[s.backendPools[index].ID] = struct{}{}
		name := strings.ToLower(s.backendPools[index].Name)
		if _, exists := poolNames[name]; exists {
			return fmt.Errorf("backend pool name %q already exists", s.backendPools[index].Name)
		}
		poolNames[name] = struct{}{}
		if err := validateBackendPool(s.backendPools[index]); err != nil {
			return fmt.Errorf("backend pool %q: %w", s.backendPools[index].ID, err)
		}
	}
	ruleIDs := make(map[string]struct{}, len(s.routingRules))
	for index := range s.routingRules {
		normalizeRoutingRule(&s.routingRules[index])
		if s.routingRules[index].ID == "" {
			return fmt.Errorf("routing rule id is required")
		}
		if _, exists := ruleIDs[s.routingRules[index].ID]; exists {
			return fmt.Errorf("routing rule id %q already exists", s.routingRules[index].ID)
		}
		ruleIDs[s.routingRules[index].ID] = struct{}{}
		if err := s.validateRoutingRuleLocked(s.routingRules[index], s.routingRules[index].ID); err != nil {
			return fmt.Errorf("routing rule %q: %w", s.routingRules[index].ID, err)
		}
	}
	return nil
}

func (s *Store) validateRoutingRuleLocked(rule model.RoutingRule, replacingID string) error {
	if rule.Name == "" {
		return fmt.Errorf("routing rule name is required")
	}
	if rule.ListenerID == "" || findListener(s.listeners, rule.ListenerID) == nil {
		return fmt.Errorf("listener id %q not found", rule.ListenerID)
	}
	if rule.BackendPoolID == "" || findBackendPool(s.backendPools, rule.BackendPoolID) == nil {
		return fmt.Errorf("backend pool id %q not found", rule.BackendPoolID)
	}
	if rule.BackendPort < 1 || rule.BackendPort > 65535 {
		return fmt.Errorf("backendPort must be between 1 and 65535")
	}
	if rule.BackendProtocol != "http" && rule.BackendProtocol != "https" {
		return fmt.Errorf("backendProtocol must be http or https")
	}
	if rule.PathPrefix != "" {
		if !strings.HasPrefix(rule.PathPrefix, "/") || strings.Contains(rule.PathPrefix, "*") {
			return fmt.Errorf("pathPrefix must start with / and must not contain wildcards")
		}
	}
	if err := validateRouteSecurity(rule.Security); err != nil {
		return err
	}
	for _, existing := range s.routingRules {
		if existing.ID != replacingID && existing.ListenerID == rule.ListenerID && existing.PathPrefix == rule.PathPrefix {
			return fmt.Errorf("routing rule for listener %q and path %q already exists", rule.ListenerID, rule.PathPrefix)
		}
	}
	return nil
}

func (s *Store) rebuildRoutesLocked() error {
	routes := make([]model.RouteConfig, 0, len(s.routingRules))
	for _, rule := range s.routingRules {
		listener := findListener(s.listeners, rule.ListenerID)
		pool := findBackendPool(s.backendPools, rule.BackendPoolID)
		if listener == nil || pool == nil {
			return fmt.Errorf("routing rule %q references missing resources", rule.ID)
		}
		upstreams := make([]model.UpstreamTarget, 0, len(pool.Targets))
		for _, target := range pool.Targets {
			port := rule.BackendPort
			if target.Port != 0 {
				port = target.Port
			}
			healthPath := rule.HealthPath
			if target.HealthPath != "" {
				healthPath = target.HealthPath
			}
			upstreams = append(upstreams, model.UpstreamTarget{
				Name:       sourceOr(target.Name, pool.Name),
				URL:        rule.BackendProtocol + "://" + hostPort(target.Address, port),
				HealthPath: healthPath,
			})
		}
		route := model.RouteConfig{
			ID:               rule.ID,
			Host:             listener.Hostname,
			ListenerPort:     listener.Port,
			ListenerProtocol: listener.Protocol,
			PathPrefix:       rule.PathPrefix,
			Exposure:         rule.Exposure,
			Enabled:          rule.Enabled,
			HTTPS:            listener.Protocol == "https",
			WebSocket:        rule.WebSocket,
			Source:           sourceOr(rule.Source, "explicit"),
			Headers:          rule.Headers,
			Security:         rule.Security,
			Upstreams:        upstreams,
			LastError:        rule.LastError,
			LastUpdated:      rule.LastUpdated,
		}
		normalize(&route)
		if err := validate(route); err != nil {
			return fmt.Errorf("compile routing rule %q: %w", rule.ID, err)
		}
		routes = append(routes, route)
	}
	s.routes = routes
	return nil
}

func (s *Store) commitLocked(snapshot storeSnapshot) error {
	if err := s.rebuildRoutesLocked(); err != nil {
		s.restoreLocked(snapshot)
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.restoreLocked(snapshot)
		return err
	}
	return nil
}

func (s *Store) snapshotLocked() storeSnapshot {
	return storeSnapshot{
		listeners:    append([]model.Listener(nil), s.listeners...),
		backendPools: cloneBackendPools(s.backendPools),
		routingRules: cloneRoutingRules(s.routingRules),
		routes:       append([]model.RouteConfig(nil), s.routes...),
	}
}

func (s *Store) restoreLocked(snapshot storeSnapshot) {
	s.listeners = snapshot.listeners
	s.backendPools = snapshot.backendPools
	s.routingRules = snapshot.routingRules
	s.routes = snapshot.routes
}

func (s *Store) pruneCompatibilityResourcesLocked(rule model.RoutingRule) {
	if !resourceReferencedByRule(s.routingRules, rule.BackendPoolID, false) {
		pool := findBackendPool(s.backendPools, rule.BackendPoolID)
		if pool != nil && pool.Source == "compatibility" {
			deleteBackendPoolByID(&s.backendPools, rule.BackendPoolID)
		}
	}
	if !resourceReferencedByRule(s.routingRules, rule.ListenerID, true) {
		listener := findListener(s.listeners, rule.ListenerID)
		if listener != nil && listener.Source == "compatibility" {
			deleteListenerByID(&s.listeners, rule.ListenerID)
		}
	}
}

func (s *Store) findMatchingListenerLocked(candidate model.Listener) *model.Listener {
	for index := range s.listeners {
		if sameListener(s.listeners[index], candidate) {
			return &s.listeners[index]
		}
	}
	return nil
}

func normalizeListener(listener *model.Listener) {
	listener.Name = strings.TrimSpace(listener.Name)
	listener.Hostname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(listener.Hostname)), ".")
	listener.Protocol = strings.ToLower(strings.TrimSpace(listener.Protocol))
	listener.Source = strings.ToLower(strings.TrimSpace(listener.Source))
	if listener.Protocol == "" {
		listener.Protocol = "https"
	}
	if listener.Port == 0 {
		listener.Port = defaultPort(listener.Protocol)
	}
}

func validateListener(listener model.Listener) error {
	if listener.Name == "" {
		return fmt.Errorf("listener name is required")
	}
	if err := validateRouteHost(listener.Hostname); err != nil {
		return err
	}
	if listener.Protocol != "http" && listener.Protocol != "https" {
		return fmt.Errorf("listener protocol must be http or https")
	}
	if listener.Port < 1 || listener.Port > 65535 {
		return fmt.Errorf("listener port must be between 1 and 65535")
	}
	return nil
}

func normalizeBackendPool(pool *model.BackendPool) {
	pool.Name = strings.TrimSpace(pool.Name)
	pool.Source = strings.ToLower(strings.TrimSpace(pool.Source))
	for index := range pool.Targets {
		pool.Targets[index].Name = strings.TrimSpace(pool.Targets[index].Name)
		pool.Targets[index].Address = strings.Trim(strings.TrimSpace(pool.Targets[index].Address), "[]")
		pool.Targets[index].HealthPath = strings.TrimSpace(pool.Targets[index].HealthPath)
	}
}

func validateBackendPool(pool model.BackendPool) error {
	if pool.Name == "" {
		return fmt.Errorf("backend pool name is required")
	}
	if len(pool.Targets) == 0 {
		return fmt.Errorf("backend pool requires at least one target")
	}
	seen := make(map[string]struct{}, len(pool.Targets))
	for _, target := range pool.Targets {
		if !validBackendAddress(target.Address) {
			return fmt.Errorf("backend target address %q must be an IP address or hostname without a port", target.Address)
		}
		if target.Port < 0 || target.Port > 65535 {
			return fmt.Errorf("backend target port must be between 1 and 65535")
		}
		key := strings.ToLower(target.Address) + ":" + strconv.Itoa(target.Port)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("backend target %q is duplicated", target.Address)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validBackendAddress(address string) bool {
	if address == "" || strings.ContainsAny(address, " /\t\r\n") {
		return false
	}
	if net.ParseIP(address) != nil {
		return true
	}
	if strings.Contains(address, ":") {
		return false
	}
	for _, label := range strings.Split(address, ".") {
		if label == "" {
			return false
		}
		for _, character := range label {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
				continue
			}
			return false
		}
	}
	return true
}

func normalizeRoutingRule(rule *model.RoutingRule) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.ListenerID = strings.TrimSpace(rule.ListenerID)
	rule.BackendPoolID = strings.TrimSpace(rule.BackendPoolID)
	rule.BackendProtocol = strings.ToLower(strings.TrimSpace(rule.BackendProtocol))
	rule.PathPrefix = strings.TrimSpace(rule.PathPrefix)
	rule.HealthPath = strings.TrimSpace(rule.HealthPath)
	rule.Exposure = strings.ToLower(strings.TrimSpace(rule.Exposure))
	rule.Source = strings.ToLower(strings.TrimSpace(rule.Source))
	if rule.PathPrefix != "/" {
		rule.PathPrefix = strings.TrimRight(rule.PathPrefix, "/")
	}
	if rule.BackendProtocol == "" {
		rule.BackendProtocol = "http"
	}
	if rule.Exposure == "" {
		rule.Exposure = "public"
	}
	rule.Security.AdditionalDeniedMethods = normalizeMethods(rule.Security.AdditionalDeniedMethods)
	rule.Security.AdditionalDeniedPathPrefixes = normalizePathPrefixes(rule.Security.AdditionalDeniedPathPrefixes)
	rule.Security.AllowedCIDRs = normalizeStrings(rule.Security.AllowedCIDRs)
	rule.Security.BlockedCIDRs = normalizeStrings(rule.Security.BlockedCIDRs)
}

func sameListener(left, right model.Listener) bool {
	return left.Hostname == right.Hostname && left.Port == right.Port && left.Protocol == right.Protocol
}

func listenerURL(listener model.Listener) string {
	return listener.Protocol + "://" + hostPort(listener.Hostname, listener.Port)
}

func hostPort(address string, port int) string {
	if strings.Contains(address, ":") {
		return net.JoinHostPort(address, strconv.Itoa(port))
	}
	return address + ":" + strconv.Itoa(port)
}

func defaultPort(protocol string) int {
	if protocol == "https" {
		return 443
	}
	return 80
}

func nextID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func resourceIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
			return character
		}
		return '-'
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "resource"
	}
	return value
}

func uniqueID(candidate string, existing map[string]struct{}) string {
	if _, ok := existing[candidate]; !ok {
		return candidate
	}
	return nextID(candidate)
}

func backendPoolIDs(pools []model.BackendPool) map[string]struct{} {
	ids := make(map[string]struct{}, len(pools))
	for _, pool := range pools {
		ids[pool.ID] = struct{}{}
	}
	return ids
}

func findListener(listeners []model.Listener, id string) *model.Listener {
	for index := range listeners {
		if listeners[index].ID == id {
			return &listeners[index]
		}
	}
	return nil
}

func findBackendPool(pools []model.BackendPool, id string) *model.BackendPool {
	for index := range pools {
		if pools[index].ID == id {
			return &pools[index]
		}
	}
	return nil
}

func findRoutingRule(rules []model.RoutingRule, id string) *model.RoutingRule {
	for index := range rules {
		if rules[index].ID == id {
			return &rules[index]
		}
	}
	return nil
}

func deleteListenerByID(listeners *[]model.Listener, id string) bool {
	for index := range *listeners {
		if (*listeners)[index].ID == id {
			*listeners = append((*listeners)[:index], (*listeners)[index+1:]...)
			return true
		}
	}
	return false
}

func deleteBackendPoolByID(pools *[]model.BackendPool, id string) bool {
	for index := range *pools {
		if (*pools)[index].ID == id {
			*pools = append((*pools)[:index], (*pools)[index+1:]...)
			return true
		}
	}
	return false
}

func deleteRoutingRuleByID(rules *[]model.RoutingRule, id string) bool {
	for index := range *rules {
		if (*rules)[index].ID == id {
			*rules = append((*rules)[:index], (*rules)[index+1:]...)
			return true
		}
	}
	return false
}

func resourceReferencedByRule(rules []model.RoutingRule, id string, listener bool) bool {
	for _, rule := range rules {
		if listener && rule.ListenerID == id || !listener && rule.BackendPoolID == id {
			return true
		}
	}
	return false
}

func cloneBackendPools(input []model.BackendPool) []model.BackendPool {
	output := append([]model.BackendPool(nil), input...)
	for index := range output {
		output[index].Targets = append([]model.BackendTarget(nil), output[index].Targets...)
	}
	return output
}

func cloneRoutingRules(input []model.RoutingRule) []model.RoutingRule {
	output := append([]model.RoutingRule(nil), input...)
	for index := range output {
		output[index].Headers = cloneStringMap(output[index].Headers)
	}
	return output
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
