package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/aidockerfarm/gateway/internal/model"
)

func Load() (model.AppConfig, error) {
	cfg := defaults()
	if path := os.Getenv("GATEWAY_CONFIG_FILE"); path != "" {
		if err := mergeFile(path, &cfg); err != nil {
			return cfg, err
		}
	}

	applyEnv(&cfg)
	normalizeConfig(&cfg)
	if cfg.Profile != model.ProfileVM && cfg.Profile != model.ProfileACI {
		return cfg, fmt.Errorf("unsupported profile %q", cfg.Profile)
	}
	if cfg.Profile == model.ProfileACI && os.Getenv("GATEWAY_DOCKER_ENABLED") == "" {
		cfg.Docker.Enabled = false
	}
	if cfg.Control.Listen == "" {
		return cfg, fmt.Errorf("control.listen is required")
	}
	if cfg.Gateway.CaddyAdminEndpoint == "" {
		return cfg, fmt.Errorf("gateway.caddyAdminEndpoint is required")
	}
	if err := validateConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func normalizeConfig(cfg *model.AppConfig) {
	cfg.Gateway.Certificate = NormalizeCertificateConfig(cfg.Gateway.Certificate)
	cfg.Auth.AdminTokens = compactStrings(cfg.Auth.AdminTokens)
	cfg.Azure.NSGSourceAddressPrefixes = compactStrings(cfg.Azure.NSGSourceAddressPrefixes)
}

func validateConfig(cfg model.AppConfig) error {
	if err := ValidateCertificateConfig(cfg.Gateway.Certificate); err != nil {
		return err
	}
	if err := validateCaddyAdminEndpoint(cfg.Gateway.CaddyAdminEndpoint); err != nil {
		return err
	}
	if (cfg.Auth.ProtectedRoutes.AdditionalHeaderName == "") != (cfg.Auth.ProtectedRoutes.AdditionalHeaderValue == "") {
		return fmt.Errorf("protected route custom header requires both name and value")
	}
	if cfg.Azure.NSGPriority != 0 && (cfg.Azure.NSGPriority < 100 || cfg.Azure.NSGPriority > 4096) {
		return fmt.Errorf("azure nsgPriority must be between 100 and 4096")
	}
	return nil
}

func validateCaddyAdminEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("gateway.caddyAdminEndpoint must be an http(s) URL with a loopback host")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("gateway.caddyAdminEndpoint must use http or https")
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("gateway.caddyAdminEndpoint must include a host")
	}
	if strings.EqualFold(hostname, "localhost") {
		return nil
	}
	ip := net.ParseIP(hostname)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("gateway.caddyAdminEndpoint must use a loopback host")
	}
	return nil
}

func NormalizeCertificateConfig(cert model.CertificateConfig) model.CertificateConfig {
	cert.Issuer = strings.ToLower(strings.TrimSpace(cert.Issuer))
	cert.Email = strings.TrimSpace(cert.Email)
	cert.CADirectory = strings.TrimSpace(cert.CADirectory)
	if cert.Issuer == "" || cert.Issuer == "default" {
		cert.Issuer = "letsencrypt"
	}
	return cert
}

func ValidateCertificateConfig(cert model.CertificateConfig) error {
	cert = NormalizeCertificateConfig(cert)
	switch cert.Issuer {
	case "default", "letsencrypt", "zerossl":
	case "custom":
		if cert.CADirectory == "" {
			return fmt.Errorf("gateway.certificate.caDirectory is required when certificate issuer is custom")
		}
	default:
		return fmt.Errorf("unsupported certificate issuer %q", cert.Issuer)
	}
	return nil
}

func defaults() model.AppConfig {
	return model.AppConfig{
		Profile: model.ProfileVM,
		Control: model.ControlConfig{Listen: ":8080"},
		Gateway: model.GatewayConfig{
			HTTPListen:         ":80",
			HTTPSListen:        ":443",
			CaddyAdminEndpoint: "http://127.0.0.1:2019",
			CaddyBin:           "caddy",
			StateDir:           "/data/platform",
			CaddyDataDir:       "/data/caddy",
			Certificate:        model.CertificateConfig{Issuer: "letsencrypt"},
		},
		Docker: model.DockerConfig{Enabled: true, SocketPath: "/var/run/docker.sock"},
		Azure:  model.AzureConfig{ManageDNS: true, ManageNSG: true, NSGPriority: 120, NSGSourceAddressPrefixes: []string{"*"}},
		Auth: model.AuthConfig{Required: true, ProtectedRoutes: model.ProtectedRouteConfig{
			AllowBearerToken:      true,
			AllowAdminTokenHeader: true,
		}},
		Health:                   model.HealthConfig{Enabled: true, TimeoutSeconds: 3, DefaultPath: "/"},
		Audit:                    model.AuditConfig{Enabled: true, File: "/data/platform/audit.jsonl"},
		RoutesFile:               envOr("GATEWAY_ROUTES_FILE", "/data/platform/routes.json"),
		ReconcileIntervalSeconds: 30,
	}
}

func mergeFile(path string, cfg *model.AppConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	return nil
}

func applyEnv(cfg *model.AppConfig) {
	if value := os.Getenv("GATEWAY_PROFILE"); value != "" {
		cfg.Profile = model.DeploymentProfile(strings.ToLower(value))
	}
	if value := os.Getenv("GATEWAY_CONTROL_LISTEN"); value != "" {
		cfg.Control.Listen = value
	}
	if value := os.Getenv("GATEWAY_MANAGEMENT_HOST"); value != "" {
		cfg.Control.ManagementHost = value
	}
	if value := os.Getenv("GATEWAY_HTTP_LISTEN"); value != "" {
		cfg.Gateway.HTTPListen = value
	}
	if value := os.Getenv("GATEWAY_HTTPS_LISTEN"); value != "" {
		cfg.Gateway.HTTPSListen = value
	}
	if value := os.Getenv("GATEWAY_CADDY_ADMIN_ENDPOINT"); value != "" {
		cfg.Gateway.CaddyAdminEndpoint = value
	}
	if value := os.Getenv("GATEWAY_CADDY_BIN"); value != "" {
		cfg.Gateway.CaddyBin = value
	}
	if value := os.Getenv("GATEWAY_STATE_DIR"); value != "" {
		cfg.Gateway.StateDir = value
	}
	if value := os.Getenv("GATEWAY_CADDY_DATA_DIR"); value != "" {
		cfg.Gateway.CaddyDataDir = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_ISSUER"); value != "" {
		cfg.Gateway.Certificate.Issuer = strings.ToLower(strings.TrimSpace(value))
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_EMAIL"); value != "" {
		cfg.Gateway.Certificate.Email = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_STAGING"); value != "" {
		cfg.Gateway.Certificate.Staging = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_CA_DIRECTORY"); value != "" {
		cfg.Gateway.Certificate.CADirectory = value
	}
	if value := os.Getenv("GATEWAY_ROUTES_FILE"); value != "" {
		cfg.RoutesFile = value
	}
	if value := os.Getenv("GATEWAY_DOCKER_SOCKET"); value != "" {
		cfg.Docker.SocketPath = value
	}
	if value := os.Getenv("GATEWAY_DOCKER_ENDPOINT"); value != "" {
		cfg.Docker.Endpoint = strings.TrimRight(value, "/")
	}
	if value := os.Getenv("GATEWAY_DOCKER_ENABLED"); value != "" {
		cfg.Docker.Enabled = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_ADMIN_TOKEN"); value != "" {
		cfg.Auth.AdminToken = value
	}
	if value := os.Getenv("GATEWAY_ADMIN_TOKENS"); value != "" {
		cfg.Auth.AdminTokens = splitCSV(value)
	}
	if value := os.Getenv("GATEWAY_PROTECTED_ALLOW_BEARER"); value != "" {
		cfg.Auth.ProtectedRoutes.AllowBearerToken = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_PROTECTED_ALLOW_ADMIN_HEADER"); value != "" {
		cfg.Auth.ProtectedRoutes.AllowAdminTokenHeader = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_PROTECTED_HEADER_NAME"); value != "" {
		cfg.Auth.ProtectedRoutes.AdditionalHeaderName = value
	}
	if value := os.Getenv("GATEWAY_PROTECTED_HEADER_VALUE"); value != "" {
		cfg.Auth.ProtectedRoutes.AdditionalHeaderValue = value
	}
	if value := os.Getenv("GATEWAY_AZURE_ENABLED"); value != "" {
		cfg.Azure.Enabled = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_AZURE_MANAGE_DNS"); value != "" {
		cfg.Azure.ManageDNS = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_AZURE_MANAGE_NSG"); value != "" {
		cfg.Azure.ManageNSG = parseBool(value)
	}
	if value := firstEnv("GATEWAY_AZURE_SUBSCRIPTION_ID", "AZURE_SUBSCRIPTION_ID"); value != "" {
		cfg.Azure.SubscriptionID = value
	}
	if value := os.Getenv("GATEWAY_AZURE_RESOURCE_GROUP"); value != "" {
		cfg.Azure.ResourceGroup = value
	}
	if value := os.Getenv("GATEWAY_AZURE_DNS_ZONE"); value != "" {
		cfg.Azure.DNSZoneName = value
	}
	if value := os.Getenv("GATEWAY_AZURE_NSG_NAME"); value != "" {
		cfg.Azure.NetworkSecurityGroupName = value
	}
	if value := os.Getenv("GATEWAY_PUBLIC_IP_ADDRESS"); value != "" {
		cfg.Azure.PublicIPAddress = value
	}
	if value := os.Getenv("GATEWAY_AZURE_NSG_PRIORITY"); value != "" {
		if priority, err := strconv.Atoi(value); err == nil && priority > 0 {
			cfg.Azure.NSGPriority = int32(priority)
		}
	}
	if value := os.Getenv("GATEWAY_AZURE_NSG_SOURCE_PREFIXES"); value != "" {
		cfg.Azure.NSGSourceAddressPrefixes = splitCSV(value)
	}
	if value := os.Getenv("GATEWAY_AUTH_REQUIRED"); value != "" {
		cfg.Auth.Required = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_HEALTH_ENABLED"); value != "" {
		cfg.Health.Enabled = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_HEALTH_TIMEOUT_SECONDS"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			cfg.Health.TimeoutSeconds = seconds
		}
	}
	if value := os.Getenv("GATEWAY_HEALTH_DEFAULT_PATH"); value != "" {
		cfg.Health.DefaultPath = value
	}
	if value := os.Getenv("GATEWAY_AUDIT_ENABLED"); value != "" {
		cfg.Audit.Enabled = parseBool(value)
	}
	if value := os.Getenv("GATEWAY_AUDIT_FILE"); value != "" {
		cfg.Audit.File = value
	}
	if value := os.Getenv("GATEWAY_RECONCILE_SECONDS"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			cfg.ReconcileIntervalSeconds = seconds
		}
	}
}

func envOr(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	return compactStrings(parts)
}

func compactStrings(parts []string) []string {
	output := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			output = append(output, part)
		}
	}
	return output
}
