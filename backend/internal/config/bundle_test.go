package config

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
)

func TestConfigurationBundleRoundTripExcludesSecretsAndCertificateMaterial(t *testing.T) {
	exportedAt := time.Date(2026, time.July, 17, 8, 30, 0, 0, time.UTC)
	settings := Settings{Auth: SettingsAuth{
		AdminToken:            "admin-secret",
		AdminTokens:           []string{"secondary-secret"},
		AdditionalHeaderName:  "X-Proxy-Token",
		AdditionalHeaderValue: "route-secret",
	}}
	certificatePolicy := model.CertificateConfig{
		Issuer: "letsencrypt",
		DNSChallenge: model.DNSChallengeConfig{
			Provider: "azure",
			Azure: model.AzureDNSChallengeConfig{
				Authentication: "appregistration",
				ClientSecret:   "azure-secret",
			},
		},
	}
	data, err := ExportConfigurationBundle(routes.ResourceSet{Version: routes.ResourceSetVersion}, settings, certificatePolicy, exportedAt)
	if err != nil {
		t.Fatalf("ExportConfigurationBundle() error = %v", err)
	}
	for _, forbidden := range []string{"admin-secret", "secondary-secret", "route-secret", "azure-secret", ".crt", ".key"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("configuration bundle contains forbidden value %q", forbidden)
		}
	}

	bundle, err := ParseConfigurationBundle(data)
	if err != nil {
		t.Fatalf("ParseConfigurationBundle() error = %v", err)
	}
	if bundle.Manifest.ExportedAt != exportedAt || bundle.Manifest.CertificateMaterialIncluded || bundle.Manifest.SecretsIncluded {
		t.Fatalf("manifest = %#v", bundle.Manifest)
	}
	if bundle.Settings.Auth.AdminToken != "" || len(bundle.Settings.Auth.AdminTokens) != 0 || bundle.Settings.Auth.AdditionalHeaderValue != "" {
		t.Fatalf("exported auth contains secrets = %#v", bundle.Settings.Auth)
	}
	if bundle.CertificatePolicy.DNSChallenge.Azure.ClientSecret != "" {
		t.Fatalf("exported certificate policy contains client secret")
	}
}

func TestParseConfigurationBundleRejectsUnexpectedPaths(t *testing.T) {
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, name := range []string{bundleManifestName, bundleRoutesName, bundleSettingsName, "../certificate.key"} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(`{}`)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := ParseConfigurationBundle(output.Bytes()); err == nil || !strings.Contains(err.Error(), "unsupported file") {
		t.Fatalf("ParseConfigurationBundle() error = %v, want unsupported file", err)
	}
}
