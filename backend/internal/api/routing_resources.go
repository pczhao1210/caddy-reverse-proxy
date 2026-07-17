package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aidockerfarm/gateway/internal/model"
)

func (s *Server) handleListeners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"listeners": s.store.Listeners()})
	case http.MethodPost:
		var listener model.Listener
		if err := json.NewDecoder(r.Body).Decode(&listener); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := s.store.AddListener(listener)
		if err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("listener.create", map[string]any{"listenerId": created.ID, "hostname": created.Hostname, "port": created.Port, "protocol": created.Protocol})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusCreated, map[string]any{"listener": created, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleListenerByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/listeners/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "listener id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var listener model.Listener
		if err := json.NewDecoder(r.Body).Decode(&listener); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		listener.ID = id
		updated, err := s.store.ReplaceListener(listener)
		if err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("listener.update", map[string]any{"listenerId": updated.ID, "hostname": updated.Hostname, "port": updated.Port, "protocol": updated.Protocol})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"listener": updated, "pendingApply": true})
	case http.MethodDelete:
		if err := s.store.DeleteListener(id); err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("listener.delete", map[string]any{"listenerId": id})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleBackendPools(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"backendPools": s.store.BackendPools()})
	case http.MethodPost:
		var pool model.BackendPool
		if err := json.NewDecoder(r.Body).Decode(&pool); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := s.store.AddBackendPool(pool)
		if err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("backend-pool.create", map[string]any{"backendPoolId": created.ID, "name": created.Name, "targets": len(created.Targets)})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusCreated, map[string]any{"backendPool": created, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleBackendPoolByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/backend-pools/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "backend pool id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var pool model.BackendPool
		if err := json.NewDecoder(r.Body).Decode(&pool); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		pool.ID = id
		updated, err := s.store.ReplaceBackendPool(pool)
		if err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("backend-pool.update", map[string]any{"backendPoolId": updated.ID, "name": updated.Name, "targets": len(updated.Targets)})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"backendPool": updated, "pendingApply": true})
	case http.MethodDelete:
		if err := s.store.DeleteBackendPool(id); err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("backend-pool.delete", map[string]any{"backendPoolId": id})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleRoutingRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"routingRules": s.store.RoutingRules()})
	case http.MethodPost:
		var rule model.RoutingRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := s.store.AddRoutingRule(rule)
		if err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("routing-rule.create", map[string]any{"routingRuleId": created.ID, "listenerId": created.ListenerID, "backendPoolId": created.BackendPoolID})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusCreated, map[string]any{"routingRule": created, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleRoutingRuleByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/routing-rules/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "routing rule id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var rule model.RoutingRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rule.ID = id
		updated, err := s.store.ReplaceRoutingRule(rule)
		if err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("routing-rule.update", map[string]any{"routingRuleId": updated.ID, "listenerId": updated.ListenerID, "backendPoolId": updated.BackendPoolID})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"routingRule": updated, "pendingApply": true})
	case http.MethodDelete:
		if err := s.store.DeleteRoutingRule(id); err != nil {
			writeRoutingResourceError(w, err)
			return
		}
		s.audit("routing-rule.delete", map[string]any{"routingRuleId": id})
		s.markRoutingChangesPending()
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id, "pendingApply": true})
	default:
		methodNotAllowed(w)
	}
}

func writeRoutingResourceError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	message := err.Error()
	if strings.Contains(message, "is used by") || strings.Contains(message, "already exists") {
		status = http.StatusConflict
	} else if strings.Contains(message, "not found") {
		status = http.StatusNotFound
	}
	writeError(w, status, message)
}
