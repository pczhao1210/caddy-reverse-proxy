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

func TestValidateConfigRejectsInvalidInternalSourceRange(t *testing.T) {
	cfg := defaults()
	cfg.Gateway.InternalSourceRanges = []string{"not-a-network"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("validateConfig() error = nil, want internal source range validation error")
	}
}

func TestApplyEnvParsesAzureDNSZones(t *testing.T) {
	t.Setenv("GATEWAY_AZURE_DNS_ZONES", `[{"name":"Example.COM.","resourceGroup":"dns-rg"},{"name":"other.net","resourceGroup":"other-rg"}]`)
	cfg := defaults()
	if err := applyEnv(&cfg); err != nil {
		t.Fatalf("applyEnv() error = %v", err)
	}
	normalizeConfig(&cfg)
	if len(cfg.Azure.DNSZones) != 2 || cfg.Azure.DNSZones[0].Name != "example.com" || cfg.Azure.DNSZones[1].ResourceGroup != "other-rg" {
		t.Fatalf("DNSZones = %#v", cfg.Azure.DNSZones)
	}
}

func TestValidateCertificateConfigAllowsAzureManagedIdentityWildcard(t *testing.T) {
	cert := model.CertificateConfig{
		Issuer:   "letsencrypt",
		Subjects: []string{"*.Example.COM.", "example.com"},
		DNSChallenge: model.DNSChallengeConfig{Provider: "azure", Azure: model.AzureDNSChallengeConfig{
			SubscriptionID: "subscription", ResourceGroup: "dns-rg", Authentication: "managedIdentity",
		}},
	}
	cert = NormalizeCertificateConfig(cert)
	if err := ValidateCertificateConfig(cert); err != nil {
		t.Fatalf("ValidateCertificateConfig() error = %v", err)
	}
	if len(cert.Subjects) != 2 || cert.Subjects[0] != "*.example.com" {
		t.Fatalf("Subjects = %#v", cert.Subjects)
	}
}

func TestValidateCertificateConfigRejectsWildcardWithoutDNSChallenge(t *testing.T) {
	cert := model.CertificateConfig{Issuer: "letsencrypt", Subjects: []string{"*.example.com"}}
	if err := ValidateCertificateConfig(cert); err == nil {
		t.Fatal("ValidateCertificateConfig() error = nil, want DNS challenge error")
	}
}

func TestValidateCertificateConfigRejectsInvalidWildcardPosition(t *testing.T) {
	cert := model.CertificateConfig{Issuer: "letsencrypt", Subjects: []string{"api.*.example.com"}}
	if err := ValidateCertificateConfig(cert); err == nil {
		t.Fatal("ValidateCertificateConfig() error = nil, want wildcard validation error")
	}
}

func TestValidateCertificateConfigRequiresAppRegistrationSecret(t *testing.T) {
	cert := model.CertificateConfig{
		Issuer:   "letsencrypt",
		Subjects: []string{"*.example.com"},
		DNSChallenge: model.DNSChallengeConfig{Provider: "azure", Azure: model.AzureDNSChallengeConfig{
			SubscriptionID: "subscription", ResourceGroup: "dns-rg", Authentication: "appRegistration", TenantID: "tenant", ClientID: "client",
		}},
	}
	if err := ValidateCertificateConfig(cert); err == nil {
		t.Fatal("ValidateCertificateConfig() error = nil, want clientSecret error")
	}
}

func TestValidateCertificateConfigRejectsZeroSSLDNSChallenge(t *testing.T) {
	cert := model.CertificateConfig{
		Issuer:   "zerossl",
		Subjects: []string{"*.example.com"},
		DNSChallenge: model.DNSChallengeConfig{Provider: "azure", Azure: model.AzureDNSChallengeConfig{
			SubscriptionID: "subscription", ResourceGroup: "dns-rg", Authentication: "managedidentity",
		}},
	}
	if err := ValidateCertificateConfig(cert); err == nil {
		t.Fatal("ValidateCertificateConfig() error = nil, want unsupported ZeroSSL DNS challenge error")
	}
}
