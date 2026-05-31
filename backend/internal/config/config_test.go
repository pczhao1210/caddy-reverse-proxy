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
