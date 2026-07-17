package azure

import "github.com/aidockerfarm/gateway/internal/model"

type Status struct {
	Enabled         bool     `json:"enabled"`
	Configured      bool     `json:"configured"`
	ManageDNS       bool     `json:"manageDNS"`
	ManageNSG       bool     `json:"manageNSG"`
	Mode            string   `json:"mode"`
	Capabilities    []string `json:"capabilities"`
	MissingSettings []string `json:"missingSettings,omitempty"`
	NextActions     []string `json:"nextActions,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

func StatusForConfig(cfg model.AppConfig) Status {
	status := Status{
		Enabled:   cfg.Azure.Enabled,
		ManageDNS: cfg.Azure.ManageDNS,
		ManageNSG: cfg.Azure.ManageNSG,
		Mode:      string(cfg.DeploymentMode),
		Capabilities: []string{
			"DefaultAzureCredential",
			"Azure DNS A record reconciliation",
		},
	}
	status.Capabilities = append(status.Capabilities, "VM NSG listener-port reconciliation")
	status.Capabilities = append(status.Capabilities, "NSG priority and source-prefix policy")
	if cfg.Azure.Enabled && cfg.Azure.SubscriptionID == "" {
		status.MissingSettings = append(status.MissingSettings, "subscriptionId")
	}
	if cfg.Azure.Enabled && cfg.Azure.ManageDNS && len(configuredDNSZones(cfg.Azure)) == 0 {
		status.MissingSettings = append(status.MissingSettings, "dnsZones")
	}
	if cfg.Azure.Enabled && cfg.Azure.ManageDNS && cfg.Azure.PublicIPAddress == "" {
		status.MissingSettings = append(status.MissingSettings, "publicIpAddress")
	}
	if cfg.Azure.Enabled && cfg.Azure.ManageNSG && cfg.Azure.NetworkSecurityGroupName == "" {
		status.MissingSettings = append(status.MissingSettings, "networkSecurityGroupName")
	}
	if cfg.Azure.Enabled && cfg.Azure.ManageNSG && nsgResourceGroup(cfg.Azure) == "" {
		status.MissingSettings = append(status.MissingSettings, "networkSecurityGroupResourceGroup")
	}
	status.Configured = cfg.Azure.Enabled && len(status.MissingSettings) == 0
	if !cfg.Azure.Enabled {
		status.Warnings = append(status.Warnings, "Azure managers are disabled")
		status.NextActions = []string{"Enable Azure integration", "Assign the required managed identity roles"}
	} else if !status.Configured {
		status.NextActions = []string{"Set the missing Azure settings, including DNS zones and the ingress public IP", "Assign the required managed identity roles"}
	} else {
		status.NextActions = []string{"Run Apply to reconcile the enabled Azure resources"}
	}
	return status
}
