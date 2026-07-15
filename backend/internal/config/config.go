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

	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}
	if certificate, found, err := NewCertificateStore(cfg.Gateway.CertificateFile).Load(); err != nil {
		return cfg, err
	} else if found {
		cfg.Gateway.Certificate = certificate
	}
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
	if cfg.Gateway.Certificate.DNSChallenge.Provider == "azure" {
		azure := &cfg.Gateway.Certificate.DNSChallenge.Azure
		if azure.SubscriptionID == "" {
			azure.SubscriptionID = strings.TrimSpace(cfg.Azure.SubscriptionID)
		}
		if azure.ResourceGroup == "" {
			azure.ResourceGroup = strings.TrimSpace(cfg.Azure.ResourceGroup)
		}
	}
	cfg.Gateway.InternalSourceRanges = compactStrings(cfg.Gateway.InternalSourceRanges)
	cfg.Auth.AdminTokens = compactStrings(cfg.Auth.AdminTokens)
	cfg.Azure.NSGSourceAddressPrefixes = compactStrings(cfg.Azure.NSGSourceAddressPrefixes)
	cfg.Azure.DNSZones = normalizeDNSZones(cfg.Azure)
}

func validateConfig(cfg model.AppConfig) error {
	if err := ValidateCertificateConfig(cfg.Gateway.Certificate); err != nil {
		return err
	}
	if err := validateCaddyAdminEndpoint(cfg.Gateway.CaddyAdminEndpoint); err != nil {
		return err
	}
	if len(cfg.Gateway.InternalSourceRanges) == 0 {
		return fmt.Errorf("gateway.internalSourceRanges must contain at least one IP or CIDR range")
	}
	for _, source := range cfg.Gateway.InternalSourceRanges {
		if net.ParseIP(source) == nil {
			if _, _, err := net.ParseCIDR(source); err != nil {
				return fmt.Errorf("gateway.internalSourceRanges contains invalid IP or CIDR %q", source)
			}
		}
	}
	if (cfg.Auth.ProtectedRoutes.AdditionalHeaderName == "") != (cfg.Auth.ProtectedRoutes.AdditionalHeaderValue == "") {
		return fmt.Errorf("protected route custom header requires both name and value")
	}
	if cfg.Azure.NSGPriority != 0 && (cfg.Azure.NSGPriority < 100 || cfg.Azure.NSGPriority > 4096) {
		return fmt.Errorf("azure nsgPriority must be between 100 and 4096")
	}
	if cfg.Azure.Enabled && cfg.Azure.ManageDNS {
		if len(cfg.Azure.DNSZones) == 0 {
			return fmt.Errorf("azure dnsZones must contain at least one DNS zone when DNS management is enabled")
		}
		for _, zone := range cfg.Azure.DNSZones {
			if zone.Name == "" || zone.ResourceGroup == "" {
				return fmt.Errorf("each azure dnsZones entry requires name and resourceGroup")
			}
		}
	}
	return nil
}

func normalizeDNSZones(cfg model.AzureConfig) []model.AzureDNSZoneConfig {
	zones := make([]model.AzureDNSZoneConfig, 0, len(cfg.DNSZones)+1)
	seen := make(map[string]struct{}, len(cfg.DNSZones)+1)
	appendZone := func(zone model.AzureDNSZoneConfig) {
		zone.Name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone.Name)), ".")
		zone.ResourceGroup = strings.TrimSpace(zone.ResourceGroup)
		if zone.ResourceGroup == "" {
			zone.ResourceGroup = strings.TrimSpace(cfg.ResourceGroup)
		}
		if zone.Name == "" {
			return
		}
		if _, ok := seen[zone.Name]; ok {
			return
		}
		seen[zone.Name] = struct{}{}
		zones = append(zones, zone)
	}
	for _, zone := range cfg.DNSZones {
		appendZone(zone)
	}
	if cfg.DNSZoneName != "" {
		appendZone(model.AzureDNSZoneConfig{Name: cfg.DNSZoneName, ResourceGroup: cfg.ResourceGroup})
	}
	return zones
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
	cert.Subjects = normalizeCertificateSubjects(cert.Subjects)
	cert.DNSChallenge.Provider = strings.ToLower(strings.TrimSpace(cert.DNSChallenge.Provider))
	cert.DNSChallenge.Azure.SubscriptionID = strings.TrimSpace(cert.DNSChallenge.Azure.SubscriptionID)
	cert.DNSChallenge.Azure.ResourceGroup = strings.TrimSpace(cert.DNSChallenge.Azure.ResourceGroup)
	cert.DNSChallenge.Azure.Authentication = strings.ToLower(strings.TrimSpace(cert.DNSChallenge.Azure.Authentication))
	cert.DNSChallenge.Azure.TenantID = strings.TrimSpace(cert.DNSChallenge.Azure.TenantID)
	cert.DNSChallenge.Azure.ClientID = strings.TrimSpace(cert.DNSChallenge.Azure.ClientID)
	cert.DNSChallenge.Azure.ClientSecret = strings.TrimSpace(cert.DNSChallenge.Azure.ClientSecret)
	if cert.Issuer == "" || cert.Issuer == "default" {
		cert.Issuer = "letsencrypt"
	}
	if cert.DNSChallenge.Provider == "azure" && cert.DNSChallenge.Azure.Authentication == "" {
		cert.DNSChallenge.Azure.Authentication = "managedidentity"
	}
	if cert.DNSChallenge.Azure.Authentication == "managedidentity" {
		cert.DNSChallenge.Azure.TenantID = ""
		cert.DNSChallenge.Azure.ClientID = ""
		cert.DNSChallenge.Azure.ClientSecret = ""
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
	for _, subject := range cert.Subjects {
		if err := validateCertificateSubject(subject); err != nil {
			return err
		}
		if strings.HasPrefix(subject, "*.") && cert.DNSChallenge.Provider == "" {
			return fmt.Errorf("wildcard certificate subject %q requires a DNS challenge", subject)
		}
	}
	switch cert.DNSChallenge.Provider {
	case "":
		return nil
	case "azure":
	default:
		return fmt.Errorf("unsupported DNS challenge provider %q", cert.DNSChallenge.Provider)
	}
	if cert.Issuer == "zerossl" {
		return fmt.Errorf("ZeroSSL does not support configurable DNS challenges; use letsencrypt or custom ACME")
	}
	if len(cert.Subjects) == 0 {
		return fmt.Errorf("DNS challenge requires at least one certificate subject")
	}
	azure := cert.DNSChallenge.Azure
	if azure.SubscriptionID == "" || azure.ResourceGroup == "" {
		return fmt.Errorf("Azure DNS challenge requires subscriptionId and resourceGroup")
	}
	switch azure.Authentication {
	case "managedidentity":
	case "appregistration":
		if azure.TenantID == "" || azure.ClientID == "" || azure.ClientSecret == "" {
			return fmt.Errorf("Azure DNS App Registration requires tenantId, clientId, and clientSecret")
		}
	default:
		return fmt.Errorf("unsupported Azure DNS authentication %q", azure.Authentication)
	}
	return nil
}

func normalizeCertificateSubjects(subjects []string) []string {
	seen := make(map[string]struct{}, len(subjects))
	output := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		subject = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(subject)), ".")
		if subject == "" {
			continue
		}
		if _, ok := seen[subject]; ok {
			continue
		}
		seen[subject] = struct{}{}
		output = append(output, subject)
	}
	return output
}

func validateCertificateSubject(subject string) error {
	wildcard := strings.HasPrefix(subject, "*.")
	name := subject
	if wildcard {
		name = strings.TrimPrefix(subject, "*.")
	}
	if strings.Contains(name, "*") || name == "" || len(name) > 253 {
		return fmt.Errorf("certificate subject %q must be a DNS name with at most one left-most wildcard", subject)
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return fmt.Errorf("certificate subject %q must be a fully qualified DNS name", subject)
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("certificate subject %q is not a valid DNS name", subject)
		}
		for _, character := range label {
			if character < 'a' || character > 'z' {
				if character < '0' || character > '9' {
					if character != '-' {
						return fmt.Errorf("certificate subject %q is not a valid DNS name", subject)
					}
				}
			}
		}
	}
	return nil
}

func defaults() model.AppConfig {
	return model.AppConfig{
		Profile: model.ProfileVM,
		Control: model.ControlConfig{Listen: ":8080"},
		Gateway: model.GatewayConfig{
			HTTPListen:           ":80",
			HTTPSListen:          ":443",
			CaddyAdminEndpoint:   "http://127.0.0.1:2019",
			CaddyBin:             "caddy",
			StateDir:             "/data/platform",
			CaddyDataDir:         "/data/caddy",
			CertificateFile:      "/data/platform/certificate.json",
			InternalSourceRanges: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "::1/128", "fc00::/7", "fe80::/10"},
			Certificate:          model.CertificateConfig{Issuer: "letsencrypt"},
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

func applyEnv(cfg *model.AppConfig) error {
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
	if value := os.Getenv("GATEWAY_CERTIFICATE_FILE"); value != "" {
		cfg.Gateway.CertificateFile = value
	}
	if value := os.Getenv("GATEWAY_INTERNAL_SOURCE_RANGES"); value != "" {
		cfg.Gateway.InternalSourceRanges = splitCSV(value)
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
	if value := os.Getenv("GATEWAY_CERTIFICATE_SUBJECTS"); value != "" {
		cfg.Gateway.Certificate.Subjects = splitCSV(value)
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_DNS_PROVIDER"); value != "" {
		cfg.Gateway.Certificate.DNSChallenge.Provider = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_AZURE_SUBSCRIPTION_ID"); value != "" {
		cfg.Gateway.Certificate.DNSChallenge.Azure.SubscriptionID = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_AZURE_RESOURCE_GROUP"); value != "" {
		cfg.Gateway.Certificate.DNSChallenge.Azure.ResourceGroup = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_AZURE_AUTHENTICATION"); value != "" {
		cfg.Gateway.Certificate.DNSChallenge.Azure.Authentication = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_AZURE_TENANT_ID"); value != "" {
		cfg.Gateway.Certificate.DNSChallenge.Azure.TenantID = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_AZURE_CLIENT_ID"); value != "" {
		cfg.Gateway.Certificate.DNSChallenge.Azure.ClientID = value
	}
	if value := os.Getenv("GATEWAY_CERTIFICATE_AZURE_CLIENT_SECRET"); value != "" {
		cfg.Gateway.Certificate.DNSChallenge.Azure.ClientSecret = value
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
	if value := os.Getenv("GATEWAY_AZURE_DNS_ZONES"); value != "" {
		var zones []model.AzureDNSZoneConfig
		if err := json.Unmarshal([]byte(value), &zones); err != nil {
			return fmt.Errorf("parse GATEWAY_AZURE_DNS_ZONES: %w", err)
		}
		cfg.Azure.DNSZones = zones
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
	return nil
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
