package config

import (
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestNormalizeCertificateConfigDefaultsToLetsEncrypt(t *testing.T) {
	tests := []struct {
		name   string
		issuer string
	}{
		{name: "empty issuer", issuer: ""},
		{name: "legacy default alias", issuer: "default"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cert := NormalizeCertificateConfig(model.CertificateConfig{Issuer: test.issuer})
			if cert.Issuer != "letsencrypt" {
				t.Fatalf("issuer = %q, want letsencrypt", cert.Issuer)
			}
		})
	}
}

func TestDefaultsUseLetsEncryptCertificateIssuer(t *testing.T) {
	cfg := defaults()
	if cfg.Gateway.Certificate.Issuer != "letsencrypt" {
		t.Fatalf("defaults certificate issuer = %q, want letsencrypt", cfg.Gateway.Certificate.Issuer)
	}
}

func TestValidateConfigAllowsLoopbackCaddyAdminEndpoint(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1:2019", "http://localhost:2019", "http://[::1]:2019"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := defaults()
			cfg.Gateway.CaddyAdminEndpoint = endpoint
			if err := validateConfig(cfg); err != nil {
				t.Fatalf("validateConfig() error = %v", err)
			}
		})
	}
}

func TestValidateConfigRejectsNonLoopbackCaddyAdminEndpoint(t *testing.T) {
	cfg := defaults()
	cfg.Gateway.CaddyAdminEndpoint = "http://0.0.0.0:2019"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("validateConfig() error = nil, want loopback validation error")
	}
}
