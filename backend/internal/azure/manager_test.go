package azure

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/aidockerfarm/gateway/internal/model"
)

type testTransport func(*http.Request) (*http.Response, error)

func (transport testTransport) Do(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func sdkResponse(request *http.Request, status int, payload any) *http.Response {
	data, _ := json.Marshal(payload)
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: request}
}

func TestInstanceIdentityIsStableAndDistinct(t *testing.T) {
	directory := t.TempDir()
	first, err := loadInstanceID(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadInstanceID(directory)
	if err != nil || first != second {
		t.Fatalf("identity changed: %q %q err=%v", first, second, err)
	}
	other, err := loadInstanceID(t.TempDir())
	if err != nil || other == first {
		t.Fatalf("independent gateways share identity: %v", err)
	}
}

func TestDNSWritesRequireOwnershipAndETag(t *testing.T) {
	for _, mode := range []string{"new", "foreign", "legacy", "unchanged", "changed", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			manager := &Manager{instanceID: "owner", cfg: model.AppConfig{Azure: model.AzureConfig{ResourceGroup: "dns-rg", DNSZoneName: "example.com"}}}
			writes := 0
			client, err := armdns.NewRecordSetsClient("subscription", testTokenCredential{}, &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: testTransport(func(request *http.Request) (*http.Response, error) {
				if request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/A") {
					return sdkResponse(request, 200, map[string]any{"value": []any{}}), nil
				}
				if request.Method == http.MethodGet {
					if mode == "new" {
						return sdkResponse(request, 404, map[string]any{"error": map[string]string{"code": "NotFound"}}), nil
					}
					metadata := manager.dnsMetadata("app.example.com")
					if mode == "foreign" {
						metadata[managedDNSOwnerKey] = to.Ptr("another-owner")
					}
					if mode == "legacy" {
						delete(metadata, managedDNSOwnerKey)
					}
					address := "203.0.113.1"
					if mode == "unchanged" {
						address = "203.0.113.2"
					}
					return sdkResponse(request, 200, armdns.RecordSet{Etag: to.Ptr("revision-one"), Properties: &armdns.RecordSetProperties{TTL: to.Ptr[int64](300), Metadata: metadata, ARecords: []*armdns.ARecord{{IPv4Address: to.Ptr(address)}}}}), nil
				}
				if request.Method != http.MethodPut {
					t.Fatalf("unexpected method %s", request.Method)
				}
				writes++
				if mode == "new" {
					if request.Header.Get("If-None-Match") != "*" {
						t.Error("new record missing create-only condition")
					}
				} else if request.Header.Get("If-Match") != "revision-one" {
					t.Error("update missing ETag condition")
				}
				if mode == "conflict" {
					return sdkResponse(request, 412, map[string]any{"error": map[string]string{"code": "PreconditionFailed"}}), nil
				}
				return sdkResponse(request, 200, map[string]any{}), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			manager.dnsClient = client
			_, _, _, err = manager.reconcileDNS(context.Background(), []model.RouteConfig{{Host: "app.example.com"}}, "203.0.113.2")
			wantError := mode == "foreign" || mode == "legacy" || mode == "conflict"
			if (err != nil) != wantError {
				t.Fatalf("error=%v expectedError=%t", err, wantError)
			}
			wantWrites := 1
			if mode == "foreign" || mode == "legacy" || mode == "unchanged" {
				wantWrites = 0
			}
			if writes != wantWrites {
				t.Fatalf("writes=%d want=%d", writes, wantWrites)
			}
		})
	}
}

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

func TestDNSCleanupOnlyDeletesOwnedRecordsConditionally(t *testing.T) {
	manager := &Manager{instanceID: "owner"}
	other := &Manager{instanceID: "other-owner"}
	deleted := []string{}
	client, err := armdns.NewRecordSetsClient("subscription", testTokenCredential{}, &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: testTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet {
			records := []*armdns.RecordSet{
				{Name: to.Ptr("mine"), Etag: to.Ptr("owned-revision"), Properties: &armdns.RecordSetProperties{Metadata: manager.dnsMetadata("mine.example.com")}},
				{Name: to.Ptr("theirs"), Etag: to.Ptr("other-revision"), Properties: &armdns.RecordSetProperties{Metadata: other.dnsMetadata("theirs.example.com")}},
				{Name: to.Ptr("legacy"), Properties: &armdns.RecordSetProperties{Metadata: managedDNSMetadata("legacy.example.com")}},
			}
			return sdkResponse(request, 200, map[string]any{"value": records}), nil
		}
		if request.Method != http.MethodDelete || !strings.HasSuffix(request.URL.Path, "/mine") || request.Header.Get("If-Match") != "owned-revision" {
			t.Fatalf("unsafe cleanup request: %s %s ETag=%q", request.Method, request.URL.Path, request.Header.Get("If-Match"))
		}
		deleted = append(deleted, request.URL.Path)
		return sdkResponse(request, 200, map[string]any{}), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	manager.dnsClient = client
	count, warnings, err := manager.cleanupDNS(context.Background(), model.AzureDNSZoneConfig{ResourceGroup: "rg", Name: "example.com"}, nil)
	if err != nil || count != 1 || len(deleted) != 1 || len(warnings) != 1 {
		t.Fatalf("cleanup: count=%d warnings=%v err=%v", count, warnings, err)
	}
}

func TestNSGOwnershipAndNoOpReconcile(t *testing.T) {
	for _, mode := range []string{"unchanged", "changed", "foreign", "delete", "foreign-delete"} {
		t.Run(mode, func(t *testing.T) {
			manager := &Manager{instanceID: "owner", cfg: model.AppConfig{Azure: model.AzureConfig{ResourceGroup: "rg", NetworkSecurityGroupName: "edge"}}}
			writes := 0
			client, err := armnetwork.NewSecurityRulesClient("subscription", testTokenCredential{}, &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: testTransport(func(request *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(request.URL.Path, "/"+manager.nsgRuleName()) {
					t.Fatalf("shared NSG rule targeted: %s", request.URL.Path)
				}
				if request.Method == http.MethodGet {
					properties := nsgRuleProperties(manager.cfg.Azure, []int{80, 443})
					properties.Description = to.Ptr(manager.nsgOwnerDescription())
					if strings.HasPrefix(mode, "foreign") {
						properties.Description = to.Ptr("another gateway")
					}
					if mode == "changed" {
						properties.Priority = to.Ptr[int32](500)
					}
					return sdkResponse(request, 200, armnetwork.SecurityRule{Properties: properties}), nil
				}
				writes++
				if request.Method == http.MethodDelete {
					return sdkResponse(request, 200, map[string]any{}), nil
				}
				if request.Method != http.MethodPut {
					t.Fatalf("unexpected method %s", request.Method)
				}
				var payload map[string]any
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				payload["properties"].(map[string]any)["provisioningState"] = "Succeeded"
				return sdkResponse(request, 200, payload), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			manager.nsgClient = client
			ports := []int{80, 443}
			if strings.HasSuffix(mode, "delete") {
				ports = nil
			}
			_, _, err = manager.reconcileNSG(context.Background(), ports)
			if (err != nil) != strings.HasPrefix(mode, "foreign") {
				t.Fatalf("unexpected error: %v", err)
			}
			wantWrites := 0
			if mode == "changed" || mode == "delete" {
				wantWrites = 1
			}
			if writes != wantWrites {
				t.Fatalf("writes=%d want=%d", writes, wantWrites)
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
