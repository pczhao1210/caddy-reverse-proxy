package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	appconfig "github.com/aidockerfarm/gateway/internal/config"
	"github.com/aidockerfarm/gateway/internal/model"
)

type protectedRouteSettings struct {
	AllowBearerToken      bool `json:"allowBearerToken"`
	AllowAdminTokenHeader bool `json:"allowAdminTokenHeader"`
}

type securitySettingsRequest struct {
	Security             model.SecurityConfig   `json:"security"`
	InternalSourceRanges []string               `json:"internalSourceRanges"`
	ProtectedRoutes      protectedRouteSettings `json:"protectedRoutes"`
}

type systemSettingsRequest struct {
	DeploymentMode model.DeploymentMode `json:"deploymentMode"`
	AdminToken     string               `json:"adminToken"`
	Azure          model.AzureConfig    `json:"azure"`
}

type azurePermissionCheckRequest struct {
	Azure model.AzureConfig `json:"azure"`
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	desired, err := s.desiredSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse(s.configSnapshot(), desired, s.settingsStore != nil))
}

func (s *Server) handleSecuritySettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings persistence is unavailable")
		return
	}
	var request securitySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	active := s.configSnapshot()
	candidate := active
	candidate.Security = request.Security
	candidate.Gateway.InternalSourceRanges = request.InternalSourceRanges
	candidate.Auth.ProtectedRoutes.AllowBearerToken = request.ProtectedRoutes.AllowBearerToken
	candidate.Auth.ProtectedRoutes.AllowAdminTokenHeader = request.ProtectedRoutes.AllowAdminTokenHeader
	if !request.ProtectedRoutes.AllowBearerToken && !request.ProtectedRoutes.AllowAdminTokenHeader && (candidate.Auth.ProtectedRoutes.AdditionalHeaderName == "" || candidate.Auth.ProtectedRoutes.AdditionalHeaderValue == "") {
		writeError(w, http.StatusBadRequest, "protected routes require at least one token header policy")
		return
	}
	if err := appconfig.NormalizeAndValidate(&candidate); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	desired, err := s.desiredSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	desired.Security = candidate.Security
	desired.InternalSourceRanges = append([]string(nil), candidate.Gateway.InternalSourceRanges...)
	desired.Auth.AllowBearerToken = candidate.Auth.ProtectedRoutes.AllowBearerToken
	desired.Auth.AllowAdminTokenHeader = candidate.Auth.ProtectedRoutes.AllowAdminTokenHeader
	if err := s.settingsStore.Save(desired); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.applyLiveConfig(candidate)
	s.audit("settings.security.update", map[string]any{"enabled": candidate.Security.Enabled})
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":  settingsResponse(candidate, desired, true),
		"reconcile": s.reconciler.Sync(r.Context()),
	})
}

func (s *Server) handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings persistence is unavailable")
		return
	}
	var request systemSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	active := s.configSnapshot()
	desired, err := s.desiredSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	desiredCandidate := active
	appconfig.ApplySettings(&desiredCandidate, desired)
	desiredCandidate.DeploymentMode = request.DeploymentMode
	desiredCandidate.Docker.Enabled = request.DeploymentMode == model.ModeContainerSocket
	desiredCandidate.Azure = request.Azure
	newToken := strings.TrimSpace(request.AdminToken)
	if newToken != "" {
		desiredCandidate.Auth.AdminToken = newToken
		desiredCandidate.Auth.AdminTokens = nil
	}
	if err := appconfig.NormalizeAndValidate(&desiredCandidate); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	desired = appconfig.SettingsFromConfig(desiredCandidate)
	if err := s.settingsStore.Save(desired); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var reconcile any
	if newToken != "" {
		active.Auth.AdminToken = newToken
		active.Auth.AdminTokens = nil
		s.applyLiveConfig(active)
		reconcile = s.reconciler.Sync(r.Context())
	}
	s.audit("settings.system.update", map[string]any{"deploymentMode": desired.DeploymentMode, "azureEnabled": desired.Azure.Enabled, "tokenRotated": newToken != ""})
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":  settingsResponse(active, desired, true),
		"reconcile": reconcile,
	})
}

func (s *Server) handleAzurePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.azurePermissionChecker == nil {
		writeError(w, http.StatusServiceUnavailable, "Azure permission checking is unavailable")
		return
	}
	var request azurePermissionCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.azurePermissionChecker.Check(r.Context(), request.Azure)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit("settings.azure.permissions.check", map[string]any{
		"dnsConfigured":     result.DNS.Configured,
		"networkConfigured": result.Network.Configured,
	})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) desiredSettings() (appconfig.Settings, error) {
	fallback := appconfig.SettingsFromConfig(s.configSnapshot())
	if s.settingsStore == nil {
		return fallback, nil
	}
	settings, found, err := s.settingsStore.Load()
	if err != nil {
		return fallback, err
	}
	if !found {
		return fallback, nil
	}
	return settings, nil
}

func (s *Server) applyLiveConfig(cfg model.AppConfig) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	if updater, ok := s.reconciler.(runtimeConfigUpdater); ok {
		updater.UpdateConfig(cfg)
	}
}

func settingsResponse(active model.AppConfig, desired appconfig.Settings, persistenceAvailable bool) map[string]any {
	tokenConfigured := desired.Auth.AdminToken != ""
	if !tokenConfigured {
		for _, token := range desired.Auth.AdminTokens {
			if strings.TrimSpace(token) != "" {
				tokenConfigured = true
				break
			}
		}
	}
	return map[string]any{
		"activeDeploymentMode": active.DeploymentMode,
		"deploymentMode":       desired.DeploymentMode,
		"azure":                desired.Azure,
		"security":             desired.Security,
		"internalSourceRanges": desired.InternalSourceRanges,
		"protectedRoutes": map[string]any{
			"allowBearerToken":      desired.Auth.AllowBearerToken,
			"allowAdminTokenHeader": desired.Auth.AllowAdminTokenHeader,
		},
		"adminTokenConfigured": tokenConfigured,
		"persistenceAvailable": persistenceAvailable,
		"restartRequired":      desired.DeploymentMode != active.DeploymentMode || !azureSettingsEqual(desired.Azure, active.Azure),
	}
}

func azureSettingsEqual(left, right model.AzureConfig) bool {
	return left.Enabled == right.Enabled &&
		left.ManageDNS == right.ManageDNS &&
		left.ManageNSG == right.ManageNSG &&
		left.SubscriptionID == right.SubscriptionID &&
		left.ResourceGroup == right.ResourceGroup &&
		left.DNSZoneName == right.DNSZoneName &&
		slices.Equal(left.DNSZones, right.DNSZones) &&
		left.NetworkSecurityGroupName == right.NetworkSecurityGroupName &&
		left.PublicIPAddress == right.PublicIPAddress &&
		left.NSGPriority == right.NSGPriority &&
		slices.Equal(left.NSGSourceAddressPrefixes, right.NSGSourceAddressPrefixes)
}
