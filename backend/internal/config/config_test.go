package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestLoadRejectsRemovedACIProfile(t *testing.T) {
	t.Setenv("GATEWAY_PROFILE", "aci")
	t.Setenv("GATEWAY_CONFIG_FILE", "")
	t.Setenv("GATEWAY_CERTIFICATE_FILE", filepath.Join(t.TempDir(), "certificate.json"))

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), `unsupported profile "aci"`) {
		t.Fatalf("Load() error = %v, want unsupported aci profile", err)
	}
}

func TestLoadVMProfileAllowsDisabledDockerDiscovery(t *testing.T) {
	t.Setenv("GATEWAY_PROFILE", "vm")
	t.Setenv("GATEWAY_DOCKER_ENABLED", "false")
	t.Setenv("GATEWAY_CONFIG_FILE", "")
	t.Setenv("GATEWAY_CERTIFICATE_FILE", filepath.Join(t.TempDir(), "certificate.json"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Docker.Enabled {
		t.Fatal("Docker discovery is enabled, want disabled for standalone VM")
	}
}

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

func TestDefaultsEnableSecurityBaseline(t *testing.T) {
	cfg := defaults()
	if !cfg.Security.Enabled || cfg.Security.MaxRequestBodyBytes != 10*1024*1024 {
		t.Fatalf("default security = %#v", cfg.Security)
	}
	if len(cfg.Security.DeniedMethods) != 2 || cfg.Security.DeniedMethods[0] != "TRACE" || cfg.Security.DeniedMethods[1] != "CONNECT" {
		t.Fatalf("default denied methods = %#v", cfg.Security.DeniedMethods)
	}
}

func TestApplyEnvParsesSecurityBaseline(t *testing.T) {
	t.Setenv("GATEWAY_SECURITY_MAX_REQUEST_BODY_BYTES", "2048")
	t.Setenv("GATEWAY_SECURITY_DENIED_METHODS", "trace, m-search")
	t.Setenv("GATEWAY_SECURITY_DENIED_PATH_PREFIXES", "/.git/, /private")
	t.Setenv("GATEWAY_SECURITY_BLOCKED_CIDRS", "192.0.2.10, 198.51.100.0/24")
	cfg := defaults()
	if err := applyEnv(&cfg); err != nil {
		t.Fatalf("applyEnv() error = %v", err)
	}
	normalizeConfig(&cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}
	if cfg.Security.MaxRequestBodyBytes != 2048 || cfg.Security.DeniedMethods[1] != "M-SEARCH" {
		t.Fatalf("security = %#v", cfg.Security)
	}
	if cfg.Security.DeniedPathPrefixes[0] != "/.git" || len(cfg.Security.BlockedCIDRs) != 2 {
		t.Fatalf("security normalization = %#v", cfg.Security)
	}
}

func TestApplyEnvCanClearSecurityLists(t *testing.T) {
	t.Setenv("GATEWAY_SECURITY_DENIED_METHODS", "")
	t.Setenv("GATEWAY_SECURITY_DENIED_PATH_PREFIXES", "")
	cfg := defaults()
	if err := applyEnv(&cfg); err != nil {
		t.Fatalf("applyEnv() error = %v", err)
	}
	if len(cfg.Security.DeniedMethods) != 0 || len(cfg.Security.DeniedPathPrefixes) != 0 {
		t.Fatalf("security lists were not cleared: %#v", cfg.Security)
	}
}

func TestValidateConfigRejectsInvalidSecurityCIDR(t *testing.T) {
	cfg := defaults()
	cfg.Security.BlockedCIDRs = []string{"not-a-network"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("validateConfig() error = nil, want security CIDR validation error")
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
