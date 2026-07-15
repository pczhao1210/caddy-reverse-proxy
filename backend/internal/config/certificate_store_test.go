package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestCertificateStoreRoundTripPreservesSecretPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "certificate.json")
	store := NewCertificateStore(path)
	want := model.CertificateConfig{
		Issuer:   "letsencrypt",
		Subjects: []string{"*.example.com"},
		DNSChallenge: model.DNSChallengeConfig{Provider: "azure", Azure: model.AzureDNSChallengeConfig{
			SubscriptionID: "subscription", ResourceGroup: "dns-rg", Authentication: "appregistration",
			TenantID: "tenant", ClientID: "client", ClientSecret: "secret",
		}},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, found, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !found || got.DNSChallenge.Azure.ClientSecret != "secret" {
		t.Fatalf("Load() = %#v, %v", got, found)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("certificate file permissions = %o, want 600", permissions)
	}
}

func TestCertificateStoreLoadMissingFile(t *testing.T) {
	store := NewCertificateStore(filepath.Join(t.TempDir(), "missing.json"))
	if _, found, err := store.Load(); err != nil || found {
		t.Fatalf("Load() found=%v error=%v, want false nil", found, err)
	}
}
