package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestMiddlewareAllowsTokenFromAllowlist(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := Middleware(model.AuthConfig{Required: true, AdminTokens: []string{"one", "two"}}, next)
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	request.Header.Set("Authorization", "Bearer two")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestMiddlewareRejectsWhenNoTokenConfigured(t *testing.T) {
	handler := Middleware(model.AuthConfig{Required: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
