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
		Mode:      string(cfg.Profile),
		Capabilities: []string{
			"DefaultAzureCredential",
			"Azure DNS A record reconciliation",
		},
	}
	if cfg.Profile == model.ProfileVM {
		status.Capabilities = append(status.Capabilities, "VM NSG inbound rule reconciliation")
		status.Capabilities = append(status.Capabilities, "NSG priority and source-prefix policy")
	} else {
		status.Capabilities = append(status.Capabilities, "ACI explicit-route mode")
		status.Warnings = append(status.Warnings, "ACI mode does not support local Docker discovery")
	}
	if cfg.Azure.Enabled && cfg.Azure.SubscriptionID == "" {
		status.MissingSettings = append(status.MissingSettings, "subscriptionId")
	}
	if cfg.Azure.Enabled && cfg.Azure.ResourceGroup == "" {
		status.MissingSettings = append(status.MissingSettings, "resourceGroup")
	}
	if cfg.Azure.Enabled && cfg.Azure.ManageDNS && cfg.Azure.DNSZoneName == "" {
		status.MissingSettings = append(status.MissingSettings, "dnsZoneName")
	}
	if cfg.Azure.Enabled && cfg.Profile == model.ProfileVM && cfg.Azure.ManageNSG && cfg.Azure.NetworkSecurityGroupName == "" {
		status.MissingSettings = append(status.MissingSettings, "networkSecurityGroupName")
	}
	status.Configured = cfg.Azure.Enabled && len(status.MissingSettings) == 0
	if !cfg.Azure.Enabled {
		status.Warnings = append(status.Warnings, "Azure managers are disabled")
	}
	if !status.Configured {
		status.NextActions = []string{"Enable Azure integration", "Assign managed identity roles", "Set subscription, resource group, DNS zone, and NSG names"}
	} else {
		status.NextActions = []string{"Run Apply to reconcile DNS records and NSG rules"}
	}
	return status
}
