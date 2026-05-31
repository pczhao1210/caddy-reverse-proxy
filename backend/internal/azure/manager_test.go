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
	properties := nsgRuleProperties(model.AzureConfig{NSGPriority: 220, NSGSourceAddressPrefixes: []string{"10.0.0.0/8", "192.168.0.0/16"}})
	if properties.Priority == nil || *properties.Priority != 220 {
		t.Fatalf("Priority = %#v", properties.Priority)
	}
	if properties.SourceAddressPrefix != nil {
		t.Fatalf("SourceAddressPrefix = %#v, want nil with multiple prefixes", *properties.SourceAddressPrefix)
	}
	if len(properties.SourceAddressPrefixes) != 2 || *properties.SourceAddressPrefixes[0] != "10.0.0.0/8" || *properties.SourceAddressPrefixes[1] != "192.168.0.0/16" {
		t.Fatalf("SourceAddressPrefixes = %#v", properties.SourceAddressPrefixes)
	}
}

func TestNSGRulePropertiesDefaultsToSharedPublicRule(t *testing.T) {
	properties := nsgRuleProperties(model.AzureConfig{})
	if properties.Priority == nil || *properties.Priority != 120 {
		t.Fatalf("Priority = %#v", properties.Priority)
	}
	if properties.SourceAddressPrefix == nil || *properties.SourceAddressPrefix != "*" {
		t.Fatalf("SourceAddressPrefix = %#v", properties.SourceAddressPrefix)
	}
}
