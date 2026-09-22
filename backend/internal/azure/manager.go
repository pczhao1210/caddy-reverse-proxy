package azure

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

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
	managedDNSOwnerKey      = "gateway-instance-id"
)

type Manager struct {
	cfg        model.AppConfig
	instanceID string
	dnsClient  *armdns.RecordSetsClient
	nsgClient  *armnetwork.SecurityRulesClient
	logger     *slog.Logger
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
	instanceID, err := loadInstanceID(cfg.Gateway.StateDir)
	if err != nil {
		return nil, fmt.Errorf("load Azure resource owner: %w", err)
	}
	manager := &Manager{cfg: cfg, logger: logger, instanceID: instanceID}
	if cfg.Azure.ManageDNS {
		client, err := armdns.NewRecordSetsClient(cfg.Azure.SubscriptionID, credential, nil)
		if err != nil {
			return nil, err
		}
		manager.dnsClient = client
	}
	if cfg.Azure.ManageNSG {
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
	if len(publicRoutes) > 0 && m.cfg.Azure.ManageDNS {
		var err error
		publicIP, err = m.publicIPAddress()
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
	if m.cfg.Azure.ManageNSG {
		rules, deleted, err := m.reconcileNSG(ctx, publicNSGPorts(publicRoutes))
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

func (m *Manager) publicIPAddress() (string, error) {
	if ip := strings.TrimSpace(m.cfg.Azure.PublicIPAddress); ip != "" {
		if parsed := net.ParseIP(ip); parsed == nil || parsed.To4() == nil {
			return "", fmt.Errorf("publicIpAddress %q must be a valid IPv4 address", ip)
		}
		return ip, nil
	}
	return "", fmt.Errorf("publicIpAddress is required when Azure DNS management has public routes")
}

func (m *Manager) reconcileDNS(ctx context.Context, routes []model.RouteConfig, publicIP string) (int, int, []string, error) {
	if m.dnsClient == nil {
		return 0, 0, nil, nil
	}
	if m.instanceID == "" {
		return 0, 0, nil, fmt.Errorf("Azure DNS owner identity is unavailable")
	}
	zones := configuredDNSZones(m.cfg.Azure)
	if len(zones) == 0 {
		return 0, 0, nil, fmt.Errorf("at least one DNS zone is required for Azure DNS reconciliation")
	}
	count := 0
	deleted := 0
	warnings := make([]string, 0)
	desiredByZone := make(map[string]map[string]struct{}, len(zones))
	for _, zone := range zones {
		desiredByZone[dnsZoneKey(zone)] = make(map[string]struct{})
	}
	for _, route := range routes {
		zone, ok := selectDNSZone(route.Host, zones)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("host %s is outside all configured DNS zones", route.Host))
			continue
		}
		relativeName, _ := relativeRecordName(route.Host, zone.Name)
		desired := desiredByZone[dnsZoneKey(zone)]
		if _, exists := desired[relativeName]; exists {
			continue
		}
		desired[relativeName] = struct{}{}
		existing, getErr := m.dnsClient.Get(ctx, zone.ResourceGroup, zone.Name, relativeName, armdns.RecordTypeA, nil)
		options := &armdns.RecordSetsClientCreateOrUpdateOptions{IfNoneMatch: to.Ptr("*")}
		if getErr == nil {
			if !m.ownsDNSRecord(&existing.RecordSet) {
				return count, deleted, warnings, fmt.Errorf("DNS record %s is not owned by this gateway; explicit ownership migration is required", route.Host)
			}
			properties := existing.Properties
			if properties.TTL != nil && *properties.TTL == 300 && len(properties.ARecords) == 1 && properties.ARecords[0] != nil && properties.ARecords[0].IPv4Address != nil && *properties.ARecords[0].IPv4Address == publicIP && properties.TargetResource == nil {
				count++
				continue
			}
			if existing.Etag == nil || *existing.Etag == "" {
				return count, deleted, warnings, fmt.Errorf("DNS record %s has no ETag; refusing unconditional update", route.Host)
			}
			options = &armdns.RecordSetsClientCreateOrUpdateOptions{IfMatch: existing.Etag}
		} else if !isNotFound(getErr) {
			return count, deleted, warnings, fmt.Errorf("read DNS record %s: %w", route.Host, getErr)
		}
		_, err := m.dnsClient.CreateOrUpdate(ctx, zone.ResourceGroup, zone.Name, relativeName, armdns.RecordTypeA, armdns.RecordSet{
			Properties: &armdns.RecordSetProperties{
				TTL:      to.Ptr[int64](300),
				Metadata: m.dnsMetadata(route.Host),
				ARecords: []*armdns.ARecord{{IPv4Address: to.Ptr(publicIP)}},
			},
		}, options)
		if err != nil {
			return count, deleted, warnings, fmt.Errorf("reconcile DNS record %s in zone %s: %w", route.Host, zone.Name, err)
		}
		count++
	}
	for _, zone := range zones {
		zoneDeleted, cleanupWarnings, err := m.cleanupDNS(ctx, zone, desiredByZone[dnsZoneKey(zone)])
		deleted += zoneDeleted
		warnings = append(warnings, cleanupWarnings...)
		if err != nil {
			return count, deleted, warnings, err
		}
	}
	return count, deleted, warnings, nil
}

func managedDNSMetadata(host string) map[string]*string {
	return map[string]*string{
		managedDNSMetadataKey: to.Ptr(managedDNSMetadataValue),
		"route-host":          to.Ptr(host),
	}
}

func (m *Manager) dnsMetadata(host string) map[string]*string {
	metadata := managedDNSMetadata(host)
	metadata[managedDNSOwnerKey] = to.Ptr(m.instanceID)
	return metadata
}

func (m *Manager) ownsDNSRecord(record *armdns.RecordSet) bool {
	if !isManagedDNSRecord(record) || m.instanceID == "" {
		return false
	}
	owner := record.Properties.Metadata[managedDNSOwnerKey]
	return owner != nil && *owner == m.instanceID
}

func (m *Manager) cleanupDNS(ctx context.Context, zone model.AzureDNSZoneConfig, desired map[string]struct{}) (int, []string, error) {
	deleted := 0
	warnings := make([]string, 0)
	pager := m.dnsClient.NewListByTypePager(zone.ResourceGroup, zone.Name, armdns.RecordTypeA, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return deleted, warnings, fmt.Errorf("list Azure DNS A records in zone %s: %w", zone.Name, err)
		}
		for _, record := range page.Value {
			if !m.ownsDNSRecord(record) {
				if isManagedDNSRecord(record) && record.Properties.Metadata[managedDNSOwnerKey] == nil {
					warnings = append(warnings, "legacy managed DNS record was retained because its instance owner is unknown")
				}
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
			if record.Etag == nil || *record.Etag == "" {
				return deleted, warnings, fmt.Errorf("DNS record %s has no ETag; refusing unconditional delete", relativeName)
			}
			if _, err := m.dnsClient.Delete(ctx, zone.ResourceGroup, zone.Name, relativeName, armdns.RecordTypeA, &armdns.RecordSetsClientDeleteOptions{IfMatch: record.Etag}); err != nil && !isNotFound(err) {
				return deleted, warnings, fmt.Errorf("delete Azure DNS record %s in zone %s: %w", relativeName, zone.Name, err)
			}
			deleted++
		}
	}
	return deleted, warnings, nil
}

func configuredDNSZones(cfg model.AzureConfig) []model.AzureDNSZoneConfig {
	zones := make([]model.AzureDNSZoneConfig, 0, len(cfg.DNSZones)+1)
	seen := make(map[string]struct{}, len(cfg.DNSZones)+1)
	appendZone := func(zone model.AzureDNSZoneConfig) {
		zone.Name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone.Name)), ".")
		zone.ResourceGroup = strings.TrimSpace(zone.ResourceGroup)
		if zone.ResourceGroup == "" {
			zone.ResourceGroup = strings.TrimSpace(cfg.ResourceGroup)
		}
		if zone.Name == "" || zone.ResourceGroup == "" {
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

func selectDNSZone(host string, zones []model.AzureDNSZoneConfig) (model.AzureDNSZoneConfig, bool) {
	var selected model.AzureDNSZoneConfig
	for _, zone := range zones {
		if _, ok := relativeRecordName(host, zone.Name); !ok {
			continue
		}
		if selected.Name == "" || len(zone.Name) > len(selected.Name) {
			selected = zone
		}
	}
	return selected, selected.Name != ""
}

func dnsZoneKey(zone model.AzureDNSZoneConfig) string {
	return zone.ResourceGroup + "\x00" + zone.Name
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

func (m *Manager) reconcileNSG(ctx context.Context, destinationPorts []int) (int, int, error) {
	if m.nsgClient == nil {
		return 0, 0, nil
	}
	if m.instanceID == "" {
		return 0, 0, fmt.Errorf("Azure NSG owner identity is unavailable")
	}
	resourceGroup := nsgResourceGroup(m.cfg.Azure)
	if resourceGroup == "" || m.cfg.Azure.NetworkSecurityGroupName == "" {
		return 0, 0, fmt.Errorf("networkSecurityGroupResourceGroup and networkSecurityGroupName are required for Azure NSG reconciliation")
	}
	if len(destinationPorts) == 0 {
		deleted, err := m.deleteNSGRule(ctx)
		return 0, deleted, err
	}
	name := m.nsgRuleName()
	properties := nsgRuleProperties(m.cfg.Azure, destinationPorts)
	properties.Description = to.Ptr(m.nsgOwnerDescription())
	existing, err := m.nsgClient.Get(ctx, resourceGroup, m.cfg.Azure.NetworkSecurityGroupName, name, nil)
	if err == nil {
		if !m.ownsNSGRule(existing.SecurityRule) {
			return 0, 0, fmt.Errorf("NSG rule %s is not owned by this gateway", name)
		}
		current := *existing.Properties
		current.ProvisioningState = nil
		if reflect.DeepEqual(current, *properties) {
			return 1, 0, nil
		}
	} else if !isNotFound(err) {
		return 0, 0, fmt.Errorf("read NSG rule %s: %w", name, err)
	}
	poller, err := m.nsgClient.BeginCreateOrUpdate(ctx, resourceGroup, m.cfg.Azure.NetworkSecurityGroupName, name, armnetwork.SecurityRule{
		Properties: properties,
	}, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("reconcile NSG rule %s: %w", name, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return 0, 0, fmt.Errorf("wait for NSG rule %s: %w", name, err)
	}
	return 1, 0, nil
}

func nsgRuleProperties(cfg model.AzureConfig, destinationPorts []int) *armnetwork.SecurityRulePropertiesFormat {
	priority := cfg.NSGPriority
	if priority <= 0 {
		priority = 120
	}
	sources := cfg.NSGSourceAddressPrefixes
	if len(sources) == 0 {
		sources = []string{"*"}
	}
	ports := make([]*string, 0, len(destinationPorts))
	for _, port := range destinationPorts {
		ports = append(ports, to.Ptr(strconv.Itoa(port)))
	}
	if len(ports) == 0 {
		ports = []*string{to.Ptr("80"), to.Ptr("443")}
	}
	properties := &armnetwork.SecurityRulePropertiesFormat{
		Description:              to.Ptr("Managed by AI Docker Farm Gateway"),
		Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
		SourcePortRange:          to.Ptr("*"),
		DestinationPortRanges:    ports,
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

func publicNSGPorts(routes []model.RouteConfig) []int {
	if len(routes) == 0 {
		return nil
	}
	unique := map[int]struct{}{80: {}, 443: {}}
	for _, route := range routes {
		if route.ListenerPort > 0 && route.ListenerPort <= 65535 {
			unique[route.ListenerPort] = struct{}{}
		}
	}
	ports := make([]int, 0, len(unique))
	for port := range unique {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
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
	name := m.nsgRuleName()
	existing, err := m.nsgClient.Get(ctx, nsgResourceGroup(m.cfg.Azure), m.cfg.Azure.NetworkSecurityGroupName, name, nil)
	if isNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read NSG rule %s: %w", name, err)
	}
	if !m.ownsNSGRule(existing.SecurityRule) {
		return 0, fmt.Errorf("NSG rule %s is not owned by this gateway", name)
	}
	poller, err := m.nsgClient.BeginDelete(ctx, nsgResourceGroup(m.cfg.Azure), m.cfg.Azure.NetworkSecurityGroupName, name, nil)
	if err != nil {
		if isNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("delete NSG rule %s: %w", name, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		if isNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("wait for NSG rule delete %s: %w", name, err)
	}
	return 1, nil
}

func (m *Manager) nsgRuleName() string {
	return managedNSGRuleName + "-" + m.instanceID
}

func (m *Manager) nsgOwnerDescription() string {
	return "Managed by AI Docker Farm Gateway; instance=" + m.instanceID
}

func (m *Manager) ownsNSGRule(rule armnetwork.SecurityRule) bool {
	return m.instanceID != "" && rule.Properties != nil && rule.Properties.Description != nil && *rule.Properties.Description == m.nsgOwnerDescription()
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
