package azure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/aidockerfarm/gateway/internal/model"
)

const managedNSGRuleName = "Allow-AIDockerFarm-Gateway-HTTPHTTPS"

const (
	managedDNSMetadataKey   = "managed-by"
	managedDNSMetadataValue = "ai-docker-farm-gateway"
)

type Manager struct {
	cfg       model.AppConfig
	dnsClient *armdns.RecordSetsClient
	nsgClient *armnetwork.SecurityRulesClient
	http      *http.Client
	logger    *slog.Logger
}

func NewManager(cfg model.AppConfig, logger *slog.Logger) (*Manager, error) {
	if !cfg.Azure.Enabled {
		return nil, nil
	}
	if cfg.Azure.SubscriptionID == "" {
		return nil, fmt.Errorf("azure subscriptionId is required when Azure is enabled")
	}
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	manager := &Manager{cfg: cfg, http: &http.Client{Timeout: 5 * time.Second}, logger: logger}
	if cfg.Azure.ManageDNS {
		client, err := armdns.NewRecordSetsClient(cfg.Azure.SubscriptionID, credential, nil)
		if err != nil {
			return nil, err
		}
		manager.dnsClient = client
	}
	if cfg.Azure.ManageNSG && cfg.Profile == model.ProfileVM {
		client, err := armnetwork.NewSecurityRulesClient(cfg.Azure.SubscriptionID, credential, nil)
		if err != nil {
			return nil, err
		}
		manager.nsgClient = client
	}
	return manager, nil
}

func (m *Manager) Reconcile(ctx context.Context, routes []model.RouteConfig) model.AzureResult {
	result := model.AzureResult{Enabled: true}
	publicRoutes := publicAzureRoutes(routes)
	if len(publicRoutes) == 0 {
		result.Warnings = append(result.Warnings, "no public routes require Azure reconciliation")
	}

	publicIP := ""
	if len(publicRoutes) > 0 {
		var err error
		publicIP, err = m.publicIPAddress(ctx)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		result.PublicIPAddress = publicIP
	}

	if m.cfg.Azure.ManageDNS {
		records, deleted, warnings, err := m.reconcileDNS(ctx, publicRoutes, publicIP)
		result.DNSRecords = records
		result.DNSDeleted = deleted
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			result.Error = err.Error()
			return result
		}
	}
	if m.cfg.Azure.ManageNSG && m.cfg.Profile == model.ProfileVM {
		rules, deleted, err := m.reconcileNSG(ctx, len(publicRoutes) > 0)
		result.NSGRules = rules
		result.NSGDeleted = deleted
		if err != nil {
			result.Error = err.Error()
			return result
		}
	}
	return result
}

func publicAzureRoutes(routes []model.RouteConfig) []model.RouteConfig {
	output := make([]model.RouteConfig, 0, len(routes))
	for _, route := range routes {
		if route.Enabled && route.Public && route.Host != "" {
			output = append(output, route)
		}
	}
	return output
}

func (m *Manager) publicIPAddress(ctx context.Context) (string, error) {
	if ip := strings.TrimSpace(m.cfg.Azure.PublicIPAddress); ip != "" {
		if parsed := net.ParseIP(ip); parsed == nil {
			return "", fmt.Errorf("publicIpAddress %q is not a valid IP address", ip)
		}
		return ip, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return "", err
	}
	response, err := m.http.Do(request)
	if err != nil {
		return "", fmt.Errorf("discover public IP: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("discover public IP returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if parsed := net.ParseIP(ip); parsed == nil {
		return "", fmt.Errorf("public IP discovery returned %q", ip)
	}
	return ip, nil
}

func (m *Manager) reconcileDNS(ctx context.Context, routes []model.RouteConfig, publicIP string) (int, int, []string, error) {
	if m.dnsClient == nil {
		return 0, 0, nil, nil
	}
	if m.cfg.Azure.ResourceGroup == "" || m.cfg.Azure.DNSZoneName == "" {
		return 0, 0, nil, fmt.Errorf("resourceGroup and dnsZoneName are required for Azure DNS reconciliation")
	}
	count := 0
	warnings := make([]string, 0)
	desired := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		relativeName, ok := relativeRecordName(route.Host, m.cfg.Azure.DNSZoneName)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("host %s is outside DNS zone %s", route.Host, m.cfg.Azure.DNSZoneName))
			continue
		}
		desired[relativeName] = struct{}{}
		_, err := m.dnsClient.CreateOrUpdate(ctx, m.cfg.Azure.ResourceGroup, m.cfg.Azure.DNSZoneName, relativeName, armdns.RecordTypeA, armdns.RecordSet{
			Properties: &armdns.RecordSetProperties{
				TTL:      to.Ptr[int64](300),
				Metadata: managedDNSMetadata(route.Host),
				ARecords: []*armdns.ARecord{{IPv4Address: to.Ptr(publicIP)}},
			},
		}, nil)
		if err != nil {
			return count, 0, warnings, fmt.Errorf("reconcile DNS record %s: %w", route.Host, err)
		}
		count++
	}
	deleted, cleanupWarnings, err := m.cleanupDNS(ctx, desired)
	warnings = append(warnings, cleanupWarnings...)
	if err != nil {
		return count, deleted, warnings, err
	}
	return count, deleted, warnings, nil
}

func managedDNSMetadata(host string) map[string]*string {
	return map[string]*string{
		managedDNSMetadataKey: to.Ptr(managedDNSMetadataValue),
		"route-host":          to.Ptr(host),
	}
}

func (m *Manager) cleanupDNS(ctx context.Context, desired map[string]struct{}) (int, []string, error) {
	deleted := 0
	warnings := make([]string, 0)
	pager := m.dnsClient.NewListByTypePager(m.cfg.Azure.ResourceGroup, m.cfg.Azure.DNSZoneName, armdns.RecordTypeA, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return deleted, warnings, fmt.Errorf("list Azure DNS A records: %w", err)
		}
		for _, record := range page.Value {
			if !isManagedDNSRecord(record) {
				continue
			}
			relativeName, ok := recordSetRelativeName(record)
			if !ok {
				warnings = append(warnings, "managed DNS record without a relative name was skipped")
				continue
			}
			if _, ok := desired[relativeName]; ok {
				continue
			}
			if _, err := m.dnsClient.Delete(ctx, m.cfg.Azure.ResourceGroup, m.cfg.Azure.DNSZoneName, relativeName, armdns.RecordTypeA, nil); err != nil && !isNotFound(err) {
				return deleted, warnings, fmt.Errorf("delete Azure DNS record %s: %w", relativeName, err)
			}
			deleted++
		}
	}
	return deleted, warnings, nil
}

func isManagedDNSRecord(record *armdns.RecordSet) bool {
	if record == nil || record.Properties == nil {
		return false
	}
	value := record.Properties.Metadata[managedDNSMetadataKey]
	return value != nil && *value == managedDNSMetadataValue
}

func recordSetRelativeName(record *armdns.RecordSet) (string, bool) {
	if record == nil || record.Name == nil {
		return "", false
	}
	name := strings.TrimSpace(*record.Name)
	if name == "" {
		return "", false
	}
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		name = parts[len(parts)-1]
	}
	return name, true
}

func (m *Manager) reconcileNSG(ctx context.Context, shouldAllowPublicTraffic bool) (int, int, error) {
	if m.nsgClient == nil {
		return 0, 0, nil
	}
	if m.cfg.Azure.ResourceGroup == "" || m.cfg.Azure.NetworkSecurityGroupName == "" {
		return 0, 0, fmt.Errorf("resourceGroup and networkSecurityGroupName are required for Azure NSG reconciliation")
	}
	if !shouldAllowPublicTraffic {
		deleted, err := m.deleteNSGRule(ctx)
		return 0, deleted, err
	}
	poller, err := m.nsgClient.BeginCreateOrUpdate(ctx, m.cfg.Azure.ResourceGroup, m.cfg.Azure.NetworkSecurityGroupName, managedNSGRuleName, armnetwork.SecurityRule{
		Properties: nsgRuleProperties(m.cfg.Azure),
	}, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("reconcile NSG rule %s: %w", managedNSGRuleName, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return 0, 0, fmt.Errorf("wait for NSG rule %s: %w", managedNSGRuleName, err)
	}
	return 1, 0, nil
}

func nsgRuleProperties(cfg model.AzureConfig) *armnetwork.SecurityRulePropertiesFormat {
	priority := cfg.NSGPriority
	if priority <= 0 {
		priority = 120
	}
	sources := cfg.NSGSourceAddressPrefixes
	if len(sources) == 0 {
		sources = []string{"*"}
	}
	properties := &armnetwork.SecurityRulePropertiesFormat{
		Description:              to.Ptr("Managed by AI Docker Farm Gateway"),
		Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
		SourcePortRange:          to.Ptr("*"),
		DestinationPortRanges:    []*string{to.Ptr("80"), to.Ptr("443")},
		DestinationAddressPrefix: to.Ptr("*"),
		Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
		Priority:                 to.Ptr(priority),
		Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
	}
	if len(sources) == 1 {
		properties.SourceAddressPrefix = to.Ptr(sources[0])
	} else {
		properties.SourceAddressPrefixes = toPtrs(sources)
	}
	return properties
}

func toPtrs(values []string) []*string {
	output := make([]*string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			output = append(output, to.Ptr(trimmed))
		}
	}
	if len(output) == 0 {
		return []*string{to.Ptr("*")}
	}
	return output
}

func (m *Manager) deleteNSGRule(ctx context.Context) (int, error) {
	poller, err := m.nsgClient.BeginDelete(ctx, m.cfg.Azure.ResourceGroup, m.cfg.Azure.NetworkSecurityGroupName, managedNSGRuleName, nil)
	if err != nil {
		if isNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("delete NSG rule %s: %w", managedNSGRuleName, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		if isNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("wait for NSG rule delete %s: %w", managedNSGRuleName, err)
	}
	return 1, nil
}

func isNotFound(err error) bool {
	var responseErr *azcore.ResponseError
	return errors.As(err, &responseErr) && responseErr.StatusCode == http.StatusNotFound
}

func relativeRecordName(host string, zone string) (string, bool) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	zone = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone)), ".")
	if host == "" || zone == "" {
		return "", false
	}
	if host == zone {
		return "@", true
	}
	suffix := "." + zone
	if strings.HasSuffix(host, suffix) {
		return strings.TrimSuffix(host, suffix), true
	}
	return "", false
}
