package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/persistence"
)

const settingsFileName = "settings.json"

type Settings struct {
	DeploymentMode       model.DeploymentMode `json:"deploymentMode"`
	DockerEnabled        *bool                `json:"dockerEnabled,omitempty"`
	Azure                model.AzureConfig    `json:"azure"`
	Security             model.SecurityConfig `json:"security"`
	InternalSourceRanges []string             `json:"internalSourceRanges"`
	Auth                 SettingsAuth         `json:"auth"`
}

type SettingsAuth struct {
	Required              bool     `json:"required"`
	AdminToken            string   `json:"adminToken,omitempty"`
	AdminTokens           []string `json:"adminTokens,omitempty"`
	AllowBearerToken      bool     `json:"allowBearerToken"`
	AllowAdminTokenHeader bool     `json:"allowAdminTokenHeader"`
	AdditionalHeaderName  string   `json:"additionalHeaderName,omitempty"`
	AdditionalHeaderValue string   `json:"additionalHeaderValue,omitempty"`
}

type SettingsStore struct {
	path string
}

func NewSettingsStore(path string) *SettingsStore {
	return &SettingsStore{path: path}
}

func SettingsPath(cfg model.AppConfig) string {
	if cfg.Gateway.StateDir == "" {
		return ""
	}
	return filepath.Join(cfg.Gateway.StateDir, settingsFileName)
}

func SettingsFromConfig(cfg model.AppConfig) Settings {
	dockerEnabled := cfg.Docker.Enabled
	return Settings{
		DeploymentMode:       cfg.DeploymentMode,
		DockerEnabled:        &dockerEnabled,
		Azure:                cfg.Azure,
		Security:             cfg.Security,
		InternalSourceRanges: append([]string(nil), cfg.Gateway.InternalSourceRanges...),
		Auth: SettingsAuth{
			Required:              cfg.Auth.Required,
			AdminToken:            cfg.Auth.AdminToken,
			AdminTokens:           append([]string(nil), cfg.Auth.AdminTokens...),
			AllowBearerToken:      cfg.Auth.ProtectedRoutes.AllowBearerToken,
			AllowAdminTokenHeader: cfg.Auth.ProtectedRoutes.AllowAdminTokenHeader,
			AdditionalHeaderName:  cfg.Auth.ProtectedRoutes.AdditionalHeaderName,
			AdditionalHeaderValue: cfg.Auth.ProtectedRoutes.AdditionalHeaderValue,
		},
	}
}

func ApplySettings(cfg *model.AppConfig, settings Settings) {
	cfg.DeploymentMode = settings.DeploymentMode
	if settings.DockerEnabled != nil {
		cfg.Docker.Enabled = *settings.DockerEnabled
	}
	cfg.Azure = settings.Azure
	cfg.Security = settings.Security
	cfg.Gateway.InternalSourceRanges = append([]string(nil), settings.InternalSourceRanges...)
	cfg.Auth.Required = settings.Auth.Required
	cfg.Auth.AdminToken = settings.Auth.AdminToken
	cfg.Auth.AdminTokens = append([]string(nil), settings.Auth.AdminTokens...)
	cfg.Auth.ProtectedRoutes.AllowBearerToken = settings.Auth.AllowBearerToken
	cfg.Auth.ProtectedRoutes.AllowAdminTokenHeader = settings.Auth.AllowAdminTokenHeader
	cfg.Auth.ProtectedRoutes.AdditionalHeaderName = settings.Auth.AdditionalHeaderName
	cfg.Auth.ProtectedRoutes.AdditionalHeaderValue = settings.Auth.AdditionalHeaderValue
}

func (s *SettingsStore) Load() (Settings, bool, error) {
	if s == nil || s.path == "" {
		return Settings{}, false, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, fmt.Errorf("read settings: %w", err)
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, false, fmt.Errorf("parse settings: %w", err)
	}
	return settings, true, nil
}

func (s *SettingsStore) Save(settings Settings) error {
	if s == nil || s.path == "" {
		return fmt.Errorf("settings file is not configured")
	}
	if err := persistence.WriteJSON(s.path, settings, 0o700); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
