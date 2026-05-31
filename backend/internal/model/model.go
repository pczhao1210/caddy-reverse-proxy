package model

import "time"

type DeploymentProfile string

const (
	ProfileVM  DeploymentProfile = "vm"
	ProfileACI DeploymentProfile = "aci"
)

type AppConfig struct {
	Profile                  DeploymentProfile `json:"profile"`
	Control                  ControlConfig     `json:"control"`
	Gateway                  GatewayConfig     `json:"gateway"`
	Docker                   DockerConfig      `json:"docker"`
	Azure                    AzureConfig       `json:"azure"`
	Auth                     AuthConfig        `json:"auth"`
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
	HTTPListen         string            `json:"httpListen"`
	HTTPSListen        string            `json:"httpsListen"`
	CaddyAdminEndpoint string            `json:"caddyAdminEndpoint"`
	CaddyBin           string            `json:"caddyBin"`
	StateDir           string            `json:"stateDir"`
	CaddyDataDir       string            `json:"caddyDataDir"`
	Certificate        CertificateConfig `json:"certificate"`
}

type CertificateConfig struct {
	Issuer      string `json:"issuer"`
	Email       string `json:"email,omitempty"`
	Staging     bool   `json:"staging"`
	CADirectory string `json:"caDirectory,omitempty"`
}

type DockerConfig struct {
	Enabled    bool   `json:"enabled"`
	SocketPath string `json:"socketPath"`
	Endpoint   string `json:"endpoint,omitempty"`
}

type AzureConfig struct {
	Enabled                  bool     `json:"enabled"`
	ManageDNS                bool     `json:"manageDNS"`
	ManageNSG                bool     `json:"manageNSG"`
	SubscriptionID           string   `json:"subscriptionId"`
	ResourceGroup            string   `json:"resourceGroup"`
	DNSZoneName              string   `json:"dnsZoneName"`
	NetworkSecurityGroupName string   `json:"networkSecurityGroupName"`
	PublicIPAddress          string   `json:"publicIpAddress"`
	NSGPriority              int32    `json:"nsgPriority"`
	NSGSourceAddressPrefixes []string `json:"nsgSourceAddressPrefixes,omitempty"`
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

type RouteConfig struct {
	ID          string            `json:"id"`
	Host        string            `json:"host"`
	PathPrefix  string            `json:"pathPrefix,omitempty"`
	Exposure    string            `json:"exposure"`
	Enabled     bool              `json:"enabled"`
	Public      bool              `json:"public"`
	HTTPS       bool              `json:"https"`
	WebSocket   bool              `json:"websocket"`
	Protected   bool              `json:"protected"`
	Source      string            `json:"source"`
	Headers     map[string]string `json:"headers,omitempty"`
	Upstreams   []UpstreamTarget  `json:"upstreams"`
	Discovered  bool              `json:"discovered"`
	LastError   string            `json:"lastError,omitempty"`
	LastUpdated time.Time         `json:"lastUpdated,omitempty"`
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
