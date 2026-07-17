package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aidockerfarm/gateway/internal/caddy"
	appconfig "github.com/aidockerfarm/gateway/internal/config"
	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
)

const maxConfigurationBundleUploadBytes = 8 << 20

type configurationImportDraft struct {
	Settings                  appconfig.Settings
	CertificatePolicy         model.CertificateConfig
	LiveConfig                model.AppConfig
	PreviousSettings          appconfig.Settings
	PreviousCertificatePolicy model.CertificateConfig
}

func (s *Server) handleConfigurationBundle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.exportConfigurationBundle(w, r)
	case http.MethodPost:
		s.importConfigurationBundle(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) exportConfigurationBundle(w http.ResponseWriter, r *http.Request) {
	if s.store == nil || s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "configuration persistence is unavailable")
		return
	}
	settings, err := s.desiredSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := s.configSnapshot()
	certificatePolicy := s.desiredCertificatePolicy(cfg.Gateway.Certificate)
	exportedAt := time.Now().UTC()
	data, err := appconfig.ExportConfigurationBundle(s.store.Resources(), settings, certificatePolicy, exportedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filename := "caddyproxy_config_" + exportedAt.Format("20060102") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	s.audit("configuration.export", map[string]any{"routes": len(s.store.RoutingRules()), "certificateSubjects": len(certificatePolicy.Subjects)})
}

func (s *Server) importConfigurationBundle(w http.ResponseWriter, r *http.Request) {
	if s.store == nil || s.settingsStore == nil || s.certificateStore == nil {
		writeError(w, http.StatusServiceUnavailable, "configuration persistence is unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigurationBundleUploadBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "configuration archive exceeds the upload size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "read configuration archive: "+err.Error())
		return
	}
	bundle, err := appconfig.ParseConfigurationBundle(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	active := s.configSnapshot()
	persistedSettings, err := s.persistedSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	secretSettings := persistedSettings
	previousCertificatePolicy := active.Gateway.Certificate
	if existing := s.configurationImportDraftSnapshot(); existing != nil {
		secretSettings = existing.Settings
		persistedSettings = existing.PreviousSettings
		previousCertificatePolicy = existing.PreviousCertificatePolicy
	}
	settings := preserveConfigurationSecrets(bundle.Settings, secretSettings)
	certificatePolicy := bundle.CertificatePolicy
	certificatePolicy.DNSChallenge.Azure.ClientSecret = s.desiredCertificatePolicy(active.Gateway.Certificate).DNSChallenge.Azure.ClientSecret
	certificatePolicy = certificateWithAzureDefaults(certificatePolicy, settings.Azure)
	certificatePolicy = appconfig.NormalizeCertificateConfig(certificatePolicy)
	if err := appconfig.ValidateCertificateConfig(certificatePolicy); err != nil {
		writeError(w, http.StatusBadRequest, "validate certificate policy: "+err.Error())
		return
	}

	candidate := active
	appconfig.ApplySettings(&candidate, settings)
	candidate.Gateway.Certificate = certificatePolicy
	if err := appconfig.NormalizeAndValidate(&candidate); err != nil {
		writeError(w, http.StatusBadRequest, "validate settings: "+err.Error())
		return
	}
	settings = appconfig.SettingsFromConfig(candidate)

	candidateStore := routes.NewStore("")
	if err := candidateStore.ReplaceResources(bundle.Routes); err != nil {
		writeError(w, http.StatusBadRequest, "validate routes: "+err.Error())
		return
	}
	liveConfig := importedLiveConfig(active, candidate)
	if _, err := caddy.NewRenderer(liveConfig).Render(candidateStore.List()); err != nil {
		writeError(w, http.StatusBadRequest, "render Caddy configuration: "+err.Error())
		return
	}

	wasPending := s.routingChangesPending()
	s.markRoutingChangesPending()
	if err := s.store.StageResources(candidateStore.Resources()); err != nil {
		if !wasPending {
			s.clearRoutingChangesPending()
		}
		writeError(w, http.StatusInternalServerError, "stage routes: "+err.Error())
		return
	}
	s.setConfigurationImportDraft(&configurationImportDraft{
		Settings:                  settings,
		CertificatePolicy:         certificatePolicy,
		LiveConfig:                liveConfig,
		PreviousSettings:          persistedSettings,
		PreviousCertificatePolicy: previousCertificatePolicy,
	})
	if updater, ok := s.reconciler.(runtimeConfigUpdater); ok {
		updater.UpdateConfig(liveConfig)
	}
	s.audit("configuration.import.staged", map[string]any{
		"listeners":           len(candidateStore.Listeners()),
		"backendPools":        len(candidateStore.BackendPools()),
		"routingRules":        len(candidateStore.RoutingRules()),
		"certificateSubjects": len(certificatePolicy.Subjects),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"pendingApply":        true,
		"listeners":           len(candidateStore.Listeners()),
		"backendPools":        len(candidateStore.BackendPools()),
		"routingRules":        len(candidateStore.RoutingRules()),
		"certificateSubjects": len(certificatePolicy.Subjects),
		"secretsPreserved":    true,
	})
}

func preserveConfigurationSecrets(imported, current appconfig.Settings) appconfig.Settings {
	imported.Auth.AdminToken = current.Auth.AdminToken
	imported.Auth.AdminTokens = append([]string(nil), current.Auth.AdminTokens...)
	imported.Auth.AdditionalHeaderName = current.Auth.AdditionalHeaderName
	imported.Auth.AdditionalHeaderValue = current.Auth.AdditionalHeaderValue
	return imported
}

func importedLiveConfig(active, imported model.AppConfig) model.AppConfig {
	live := active
	live.Security = imported.Security
	live.Gateway.InternalSourceRanges = append([]string(nil), imported.Gateway.InternalSourceRanges...)
	live.Gateway.Certificate = imported.Gateway.Certificate
	live.Auth = imported.Auth
	return live
}

func (s *Server) configurationImportDraftSnapshot() *configurationImportDraft {
	s.configurationMu.Lock()
	defer s.configurationMu.Unlock()
	if s.configurationDraft == nil {
		return nil
	}
	draft := *s.configurationDraft
	return &draft
}

func (s *Server) setConfigurationImportDraft(draft *configurationImportDraft) {
	s.configurationMu.Lock()
	s.configurationDraft = draft
	s.configurationMu.Unlock()
}

func (s *Server) configurationImportPending() bool {
	return s.configurationImportDraftSnapshot() != nil
}

func (s *Server) rejectWhileConfigurationImportPending(w http.ResponseWriter) bool {
	if !s.configurationImportPending() {
		return false
	}
	writeError(w, http.StatusConflict, "apply the imported configuration before changing settings or certificate policy")
	return true
}

func (s *Server) desiredCertificatePolicy(fallback model.CertificateConfig) model.CertificateConfig {
	if draft := s.configurationImportDraftSnapshot(); draft != nil {
		return draft.CertificatePolicy
	}
	return fallback
}

func (s *Server) persistConfigurationImport() error {
	s.configurationMu.Lock()
	defer s.configurationMu.Unlock()
	if s.configurationDraft == nil {
		return nil
	}
	draft := *s.configurationDraft
	if err := s.settingsStore.Save(draft.Settings); err != nil {
		return err
	}
	if err := s.certificateStore.Save(draft.CertificatePolicy); err != nil {
		rollbackErr := s.settingsStore.Save(draft.PreviousSettings)
		return errors.Join(err, rollbackErr)
	}
	if err := s.store.PersistStaged(); err != nil {
		rollbackErr := errors.Join(
			s.settingsStore.Save(draft.PreviousSettings),
			s.certificateStore.Save(draft.PreviousCertificatePolicy),
		)
		return errors.Join(err, rollbackErr)
	}
	s.configurationDraft = nil
	s.applyLiveConfig(draft.LiveConfig)
	s.audit("configuration.import.applied", map[string]any{"routingRules": len(s.store.RoutingRules()), "certificateSubjects": len(draft.CertificatePolicy.Subjects)})
	return nil
}
