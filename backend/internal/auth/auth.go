package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aidockerfarm/gateway/internal/model"
)

func Middleware(cfg model.AuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Required {
			next.ServeHTTP(w, r)
			return
		}
		if !tokensConfigured(cfg) {
			writeAuthError(w, http.StatusServiceUnavailable, "admin token is not configured")
			return
		}
		provided := tokenFromRequest(r)
		if provided == "" || !tokenAllowed(cfg, provided) {
			writeAuthError(w, http.StatusUnauthorized, "invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func tokensConfigured(cfg model.AuthConfig) bool {
	if cfg.AdminToken != "" {
		return true
	}
	for _, token := range cfg.AdminTokens {
		if token != "" {
			return true
		}
	}
	return false
}

func tokenAllowed(cfg model.AuthConfig, provided string) bool {
	if cfg.AdminToken != "" && provided == cfg.AdminToken {
		return true
	}
	for _, token := range cfg.AdminTokens {
		if token != "" && provided == token {
			return true
		}
	}
	return false
}

func tokenFromRequest(r *http.Request) string {
	if value := r.Header.Get("X-Admin-Token"); value != "" {
		return value
	}
	value := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
