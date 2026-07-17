package model

import (
	"encoding/json"
	"time"
)

type DeploymentProfile string
type DeploymentMode string

const (
	ProfileVM           DeploymentProfile = "vm"
	ModeContainerSocket DeploymentMode    = "container-socket"
	ModeAzureVM         DeploymentMode    = "azure-vm"
)

type AppConfig struct {
	Profile                  DeploymentProfile `json:"profile"`
	DeploymentMode           DeploymentMode    `json:"deploymentMode"`
	Control                  ControlConfig     `json:"control"`
	Gateway                  GatewayConfig     `json:"gateway"`
	Docker                   DockerConfig      `json:"docker"`
	Azure                    AzureConfig       `json:"azure"`
	Auth                     AuthConfig        `json:"auth"`
	Security                 SecurityConfig    `json:"security"`
	Health                   HealthConfig      `json:"health"`
	Audit                    AuditConfig       `json:"audit"`
	RoutesFile               string            `json:"routesFile"`
	ReconcileIntervalSeconds int               `json:"reconcileIntervalSeconds"`
}

type ControlConfig struct {
	Listen         string `json:"listen"`
	ManagementHost string `json:"managementHost"`
}

type GatewayConfig struct {
	HTTPListen           string            `json:"httpListen"`
	HTTPSListen          string            `json:"httpsListen"`
	CaddyAdminEndpoint   string            `json:"caddyAdminEndpoint"`
	CaddyBin             string            `json:"caddyBin"`
	StateDir             string            `json:"stateDir"`
	CaddyDataDir         string            `json:"caddyDataDir"`
	CertificateFile      string            `json:"certificateFile"`
	InternalSourceRanges []string          `json:"internalSourceRanges,omitempty"`
	Certificate          CertificateConfig `json:"certificate"`
}

type CertificateConfig struct {
	Issuer             string             `json:"issuer"`
	Email              string             `json:"email,omitempty"`
	Staging            bool               `json:"staging"`
	CADirectory        string             `json:"caDirectory,omitempty"`
	Subjects           []string           `json:"subjects,omitempty"`
	RenewalWindowRatio float64            `json:"renewalWindowRatio,omitempty"`
	DNSChallenge       DNSChallengeConfig `json:"dnsChallenge,omitempty"`
}

type DNSChallengeConfig struct {
	Provider string                  `json:"provider,omitempty"`
	Azure    AzureDNSChallengeConfig `json:"azure,omitempty"`
}

type AzureDNSChallengeConfig struct {
	SubscriptionID string `json:"subscriptionId,omitempty"`
	ResourceGroup  string `json:"resourceGroup,omitempty"`
	Authentication string `json:"authentication,omitempty"`
	TenantID       string `json:"tenantId,omitempty"`
	ClientID       string `json:"clientId,omitempty"`
	ClientSecret   string `json:"clientSecret,omitempty"`
}

type DockerConfig struct {
	Enabled    bool   `json:"enabled"`
	SocketPath string `json:"socketPath"`
	Endpoint   string `json:"endpoint,omitempty"`
}

type AzureConfig struct {
	Enabled                           bool                 `json:"enabled"`
	ManageDNS                         bool                 `json:"manageDNS"`
	ManageNSG                         bool                 `json:"manageNSG"`
	SubscriptionID                    string               `json:"subscriptionId"`
	ResourceGroup                     string               `json:"resourceGroup"`
	DNSZoneName                       string               `json:"dnsZoneName"`
	DNSZones                          []AzureDNSZoneConfig `json:"dnsZones,omitempty"`
	NetworkSecurityGroupResourceGroup string               `json:"networkSecurityGroupResourceGroup"`
	NetworkSecurityGroupName          string               `json:"networkSecurityGroupName"`
	PublicIPAddress                   string               `json:"publicIpAddress"`
	NSGPriority                       int32                `json:"nsgPriority"`
	NSGSourceAddressPrefixes          []string             `json:"nsgSourceAddressPrefixes,omitempty"`
}

type AzureDNSZoneConfig struct {
	Name          string `json:"name"`
	ResourceGroup string `json:"resourceGroup"`
}

type AuthConfig struct {
	Required        bool                 `json:"required"`
	AdminToken      string               `json:"-"`
	AdminTokens     []string             `json:"-"`
	ProtectedRoutes ProtectedRouteConfig `json:"protectedRoutes"`
}

type ProtectedRouteConfig struct {
	AllowBearerToken      bool   `json:"allowBearerToken"`
	AllowAdminTokenHeader bool   `json:"allowAdminTokenHeader"`
	AdditionalHeaderName  string `json:"additionalHeaderName,omitempty"`
	AdditionalHeaderValue string `json:"-"`
}

type SecurityConfig struct {
	Enabled             bool     `json:"enabled"`
	MaxRequestBodyBytes int64    `json:"maxRequestBodyBytes"`
	DeniedMethods       []string `json:"deniedMethods,omitempty"`
	DeniedPathPrefixes  []string `json:"deniedPathPrefixes,omitempty"`
	AllowedCIDRs        []string `json:"allowedCidrs,omitempty"`
	BlockedCIDRs        []string `json:"blockedCidrs,omitempty"`
}

type RouteSecurityConfig struct {
	Disabled                     bool     `json:"disabled,omitempty"`
	MaxRequestBodyBytes          int64    `json:"maxRequestBodyBytes,omitempty"`
	AdditionalDeniedMethods      []string `json:"additionalDeniedMethods,omitempty"`
	AdditionalDeniedPathPrefixes []string `json:"additionalDeniedPathPrefixes,omitempty"`
	AllowedCIDRs                 []string `json:"allowedCidrs,omitempty"`
	BlockedCIDRs                 []string `json:"blockedCidrs,omitempty"`
}

type HealthConfig struct {
	Enabled        bool   `json:"enabled"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	DefaultPath    string `json:"defaultPath,omitempty"`
}

type AuditConfig struct {
	Enabled bool   `json:"enabled"`
	File    string `json:"file"`
}

type UpstreamTarget struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	HealthPath string `json:"healthPath,omitempty"`
}

type Listener struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Hostname    string    `json:"hostname"`
	Port        int       `json:"port"`
	Protocol    string    `json:"protocol"`
	Source      string    `json:"source"`
	LastUpdated time.Time `json:"lastUpdated,omitempty"`
}

type BackendTarget struct {
	Name       string `json:"name,omitempty"`
	Address    string `json:"address"`
	Port       int    `json:"port,omitempty"`
	HealthPath string `json:"healthPath,omitempty"`
}

type BackendPool struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Targets     []BackendTarget `json:"targets"`
	Source      string          `json:"source"`
	LastUpdated time.Time       `json:"lastUpdated,omitempty"`
}

type RoutingRule struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	ListenerID      string              `json:"listenerId"`
	BackendPoolID   string              `json:"backendPoolId"`
	BackendPort     int                 `json:"backendPort"`
	BackendProtocol string              `json:"backendProtocol"`
	PathPrefix      string              `json:"pathPrefix,omitempty"`
	HealthPath      string              `json:"healthPath,omitempty"`
	Exposure        string              `json:"exposure"`
	Enabled         bool                `json:"enabled"`
	WebSocket       bool                `json:"websocket"`
	Headers         map[string]string   `json:"headers,omitempty"`
	Security        RouteSecurityConfig `json:"security,omitempty"`
	Source          string              `json:"source"`
	LastError       string              `json:"-"`
	LastUpdated     time.Time           `json:"lastUpdated,omitempty"`
}

func (rule *RoutingRule) UnmarshalJSON(data []byte) error {
	type routingRule RoutingRule
	type routingRulePayload struct {
		routingRule
		Enabled *bool `json:"enabled"`
	}
	payload := routingRulePayload{routingRule: routingRule{Enabled: true}}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	output := RoutingRule(payload.routingRule)
	if payload.Enabled != nil {
		output.Enabled = *payload.Enabled
	}
	*rule = output
	return nil
}

type RouteConfig struct {
	ID               string              `json:"id"`
	Host             string              `json:"host"`
	ListenerPort     int                 `json:"listenerPort,omitempty"`
	ListenerProtocol string              `json:"listenerProtocol,omitempty"`
	PathPrefix       string              `json:"pathPrefix,omitempty"`
	Exposure         string              `json:"exposure"`
	Enabled          bool                `json:"enabled"`
	Public           bool                `json:"public"`
	HTTPS            bool                `json:"https"`
	WebSocket        bool                `json:"websocket"`
	Protected        bool                `json:"protected"`
	Source           string              `json:"source"`
	Headers          map[string]string   `json:"headers,omitempty"`
	Security         RouteSecurityConfig `json:"security,omitempty"`
	Upstreams        []UpstreamTarget    `json:"upstreams"`
	Discovered       bool                `json:"discovered"`
	LastError        string              `json:"lastError,omitempty"`
	LastUpdated      time.Time           `json:"lastUpdated,omitempty"`
}

func (route *RouteConfig) UnmarshalJSON(data []byte) error {
	type routeConfig RouteConfig
	type routePayload struct {
		routeConfig
		Enabled *bool `json:"enabled"`
	}
	payload := routePayload{routeConfig: routeConfig{Enabled: true}}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	output := RouteConfig(payload.routeConfig)
	if payload.Enabled != nil {
		output.Enabled = *payload.Enabled
	}
	*route = output
	return nil
}

type ContainerService struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Image            string               `json:"image"`
	State            string               `json:"state"`
	Status           string               `json:"status"`
	Ports            []ContainerPort      `json:"ports"`
	Labels           map[string]string    `json:"labels"`
	Networks         []string             `json:"networks"`
	NetworkEndpoints []NetworkEndpoint    `json:"networkEndpoints,omitempty"`
	NetworkAddresses []string             `json:"networkAddresses,omitempty"`
	BindPolicy       *ContainerBindPolicy `json:"bindPolicy,omitempty"`
	RouteHint        *RouteConfig         `json:"routeHint,omitempty"`
}

type NetworkEndpoint struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

type ContainerBindPolicy struct {
	CanBind           bool     `json:"canBind"`
	Mode              string   `json:"mode"`
	SharedNetworks    []string `json:"sharedNetworks,omitempty"`
	GatewayNetworks   []string `json:"gatewayNetworks,omitempty"`
	SuggestedUpstream string   `json:"suggestedUpstream,omitempty"`
}

type ContainerPort struct {
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort,omitempty"`
	Type        string `json:"type"`
}

type ReconcileResult struct {
	StartedAt        time.Time           `json:"startedAt"`
	FinishedAt       time.Time           `json:"finishedAt"`
	Duration         time.Duration       `json:"duration"`
	Profile          string              `json:"profile"`
	ExplicitRoutes   int                 `json:"explicitRoutes"`
	DiscoveredRoutes int                 `json:"discoveredRoutes"`
	AppliedRoutes    int                 `json:"appliedRoutes"`
	HealthChecks     int                 `json:"healthChecks"`
	UnhealthyRoutes  int                 `json:"unhealthyRoutes"`
	CaddyLoaded      bool                `json:"caddyLoaded"`
	RouteHealth      []RouteHealthStatus `json:"routeHealth,omitempty"`
	Azure            AzureResult         `json:"azure"`
	Error            string              `json:"error,omitempty"`
}

type RouteHealthStatus struct {
	RouteID   string    `json:"routeId"`
	Host      string    `json:"host"`
	Healthy   bool      `json:"healthy"`
	CheckedAt time.Time `json:"checkedAt"`
	Error     string    `json:"error,omitempty"`
}

type AzureResult struct {
	Enabled         bool     `json:"enabled"`
	DNSRecords      int      `json:"dnsRecords"`
	DNSDeleted      int      `json:"dnsDeleted"`
	NSGRules        int      `json:"nsgRules"`
	NSGDeleted      int      `json:"nsgDeleted"`
	PublicIPAddress string   `json:"publicIpAddress,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	Error           string   `json:"error,omitempty"`
}
