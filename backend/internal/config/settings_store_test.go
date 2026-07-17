package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestSettingsStoreRoundTripPreservesPrivateSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := NewSettingsStore(path)
	want := SettingsFromConfig(model.AppConfig{
		DeploymentMode: model.ModeAzureVM,
		Gateway:        model.GatewayConfig{InternalSourceRanges: []string{"10.0.0.0/8"}},
		Auth: model.AuthConfig{
			Required:   true,
			AdminToken: "rotated-token",
			ProtectedRoutes: model.ProtectedRouteConfig{
				AllowBearerToken: true,
			},
		},
		Security: model.SecurityConfig{Enabled: true, DeniedMethods: []string{"TRACE"}},
	})
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, found, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !found || got.DeploymentMode != model.ModeAzureVM || got.Auth.AdminToken != "rotated-token" || len(got.InternalSourceRanges) != 1 {
		t.Fatalf("Load() = %#v, found %t", got, found)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("settings permissions = %o, want 600", permissions)
	}
}
