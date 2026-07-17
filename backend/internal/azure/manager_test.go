package azure

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/aidockerfarm/gateway/internal/model"
)

func TestRelativeRecordName(t *testing.T) {
	tests := []struct {
		name string
		host string
		zone string
		want string
		ok   bool
	}{
		{name: "zone apex", host: "example.com", zone: "example.com", want: "@", ok: true},
		{name: "subdomain", host: "app.example.com", zone: "example.com", want: "app", ok: true},
		{name: "nested subdomain", host: "api.dev.example.com", zone: "example.com", want: "api.dev", ok: true},
		{name: "outside zone", host: "app.contoso.com", zone: "example.com", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := relativeRecordName(test.host, test.zone)
			if ok != test.ok || got != test.want {
				t.Fatalf("relativeRecordName(%q, %q) = %q, %v; want %q, %v", test.host, test.zone, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestPublicAzureRoutesExcludesInternalAndDisabled(t *testing.T) {
	routes := []model.RouteConfig{
		{Host: "public.example.com", Enabled: true, Public: true},
		{Host: "protected.example.com", Enabled: true, Public: true, Protected: true},
		{Host: "internal.example.com", Enabled: true, Public: false},
		{Host: "disabled.example.com", Enabled: false, Public: true},
	}
	got := publicAzureRoutes(routes)
	if len(got) != 2 {
		t.Fatalf("len(publicAzureRoutes) = %d, want 2", len(got))
	}
	if got[0].Host != "public.example.com" || got[1].Host != "protected.example.com" {
		t.Fatalf("publicAzureRoutes = %#v", got)
	}
}

func TestManagedDNSRecordDetection(t *testing.T) {
	record := &armdns.RecordSet{Properties: &armdns.RecordSetProperties{Metadata: managedDNSMetadata("app.example.com")}}
	if !isManagedDNSRecord(record) {
		t.Fatal("isManagedDNSRecord() = false, want true")
	}
	unmanaged := &armdns.RecordSet{Properties: &armdns.RecordSetProperties{Metadata: map[string]*string{managedDNSMetadataKey: to.Ptr("someone-else")}}}
	if isManagedDNSRecord(unmanaged) {
		t.Fatal("isManagedDNSRecord() = true for unmanaged record")
	}
}

func TestRecordSetRelativeName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "app", want: "app"},
		{name: "subscriptions/123/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com/A/app", want: "app"},
	}
	for _, test := range tests {
		got, ok := recordSetRelativeName(&armdns.RecordSet{Name: to.Ptr(test.name)})
		if !ok || got != test.want {
			t.Fatalf("recordSetRelativeName(%q) = %q, %v; want %q, true", test.name, got, ok, test.want)
		}
	}
}

func TestNSGRulePropertiesUsesPriorityAndSourcePrefixes(t *testing.T) {
	properties := nsgRuleProperties(model.AzureConfig{NSGPriority: 220, NSGSourceAddressPrefixes: []string{"10.0.0.0/8", "192.168.0.0/16"}}, []int{80, 443, 8443})
	if properties.Priority == nil || *properties.Priority != 220 {
		t.Fatalf("Priority = %#v", properties.Priority)
	}
	if properties.SourceAddressPrefix != nil {
		t.Fatalf("SourceAddressPrefix = %#v, want nil with multiple prefixes", *properties.SourceAddressPrefix)
	}
	if len(properties.SourceAddressPrefixes) != 2 || *properties.SourceAddressPrefixes[0] != "10.0.0.0/8" || *properties.SourceAddressPrefixes[1] != "192.168.0.0/16" {
		t.Fatalf("SourceAddressPrefixes = %#v", properties.SourceAddressPrefixes)
	}
	if len(properties.DestinationPortRanges) != 3 || *properties.DestinationPortRanges[2] != "8443" {
		t.Fatalf("DestinationPortRanges = %#v", properties.DestinationPortRanges)
	}
}

func TestNSGRulePropertiesDefaultsToSharedPublicRule(t *testing.T) {
	properties := nsgRuleProperties(model.AzureConfig{}, nil)
	if properties.Priority == nil || *properties.Priority != 120 {
		t.Fatalf("Priority = %#v", properties.Priority)
	}
	if properties.SourceAddressPrefix == nil || *properties.SourceAddressPrefix != "*" {
		t.Fatalf("SourceAddressPrefix = %#v", properties.SourceAddressPrefix)
	}
}

func TestPublicNSGPortsIncludesCustomListenersAndACMEPorts(t *testing.T) {
	ports := publicNSGPorts([]model.RouteConfig{
		{ListenerPort: 8443},
		{ListenerPort: 8080},
		{ListenerPort: 8443},
	})
	want := []int{80, 443, 8080, 8443}
	if len(ports) != len(want) {
		t.Fatalf("publicNSGPorts() = %#v, want %#v", ports, want)
	}
	for index := range want {
		if ports[index] != want[index] {
			t.Fatalf("publicNSGPorts() = %#v, want %#v", ports, want)
		}
	}
	if ports := publicNSGPorts(nil); ports != nil {
		t.Fatalf("publicNSGPorts(nil) = %#v, want nil", ports)
	}
}

func TestSelectDNSZonePrefersLongestSuffix(t *testing.T) {
	zones := []model.AzureDNSZoneConfig{
		{Name: "example.com", ResourceGroup: "root-rg"},
		{Name: "dev.example.com", ResourceGroup: "dev-rg"},
		{Name: "other.net", ResourceGroup: "other-rg"},
	}
	zone, ok := selectDNSZone("api.dev.example.com", zones)
	if !ok || zone.Name != "dev.example.com" || zone.ResourceGroup != "dev-rg" {
		t.Fatalf("selectDNSZone() = %#v, %v", zone, ok)
	}
	if _, ok := selectDNSZone("unknown.org", zones); ok {
		t.Fatal("selectDNSZone() matched host outside configured zones")
	}
}

func TestConfiguredDNSZonesSupportsLegacyAndStructuredConfig(t *testing.T) {
	zones := configuredDNSZones(model.AzureConfig{
		ResourceGroup: "legacy-rg",
		DNSZoneName:   "example.com",
		DNSZones:      []model.AzureDNSZoneConfig{{Name: "other.net", ResourceGroup: "other-rg"}},
	})
	if len(zones) != 2 || zones[0].Name != "other.net" || zones[1].Name != "example.com" {
		t.Fatalf("configuredDNSZones() = %#v", zones)
	}
}

func TestPublicIPAddressMustBeExplicit(t *testing.T) {
	manager := &Manager{cfg: model.AppConfig{Azure: model.AzureConfig{}}}
	if _, err := manager.publicIPAddress(); err == nil {
		t.Fatal("publicIPAddress() error = nil, want explicit public IP requirement")
	}
	manager.cfg.Azure.PublicIPAddress = "203.0.113.10"
	if got, err := manager.publicIPAddress(); err != nil || got != "203.0.113.10" {
		t.Fatalf("publicIPAddress() = %q, %v", got, err)
	}
	manager.cfg.Azure.PublicIPAddress = "2001:db8::10"
	if _, err := manager.publicIPAddress(); err == nil {
		t.Fatal("publicIPAddress() error = nil for IPv6 address")
	}
}
