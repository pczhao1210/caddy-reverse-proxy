package azure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/aidockerfarm/gateway/internal/model"
)

type testTokenCredential struct{}

func (testTokenCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "test-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestMissingActionsHonorsNotActions(t *testing.T) {
	permissions := []permissionBlock{{
		actions:    []string{"Microsoft.Network/dnsZones/*"},
		notActions: []string{"Microsoft.Network/dnsZones/A/delete"},
	}}
	missing := missingActions(dnsRequiredActions, permissions)
	if len(missing) != 1 || missing[0] != "Microsoft.Network/dnsZones/A/delete" {
		t.Fatalf("missingActions() = %#v, want DNS A delete", missing)
	}
}

func TestActionExcludedByOnePermissionBlockCanBeGrantedByAnother(t *testing.T) {
	permissions := []permissionBlock{
		{
			actions:    []string{"Microsoft.Network/networkSecurityGroups/securityRules/*"},
			notActions: []string{"*/delete"},
		},
		{actions: []string{"Microsoft.Network/networkSecurityGroups/securityRules/delete"}},
	}
	if missing := missingActions(networkRequiredActions, permissions); len(missing) != 0 {
		t.Fatalf("missingActions() = %#v, want no missing actions", missing)
	}
}

func TestAzureActionMatchesWildcardsCaseInsensitively(t *testing.T) {
	tests := []struct {
		pattern string
		action  string
		want    bool
	}{
		{pattern: "*", action: "Microsoft.Network/dnsZones/A/write", want: true},
		{pattern: "microsoft.network/dnszones/*", action: "Microsoft.Network/dnsZones/A/read", want: true},
		{pattern: "*/read", action: "Microsoft.Network/networkSecurityGroups/securityRules/read", want: true},
		{pattern: "Microsoft.Network/dnsZones/*", action: "Microsoft.Network/networkSecurityGroups/read", want: false},
	}
	for _, test := range tests {
		if got := azureActionMatches(test.pattern, test.action); got != test.want {
			t.Errorf("azureActionMatches(%q, %q) = %v, want %v", test.pattern, test.action, got, test.want)
		}
	}
}

func TestAzureResourceScope(t *testing.T) {
	target := permissionTarget{name: "edge-nsg", resourceGroup: "network-rg", provider: "Microsoft.Network", resourceType: "networkSecurityGroups"}
	want := "/subscriptions/sub-id/resourceGroups/network-rg/providers/Microsoft.Network/networkSecurityGroups/edge-nsg"
	if got := azureResourceScope("sub-id", target); got != want {
		t.Fatalf("azureResourceScope() = %q, want %q", got, want)
	}
}

func TestPermissionCheckerReadsResourcePermissionsAcrossPages(t *testing.T) {
	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path == "/next" {
			io.WriteString(w, `{"value":[{"actions":["Microsoft.Network/dnsZones/A/delete"]}]}`)
			return
		}
		wantPath := "/subscriptions/sub-id/resourceGroups/dns-rg/providers/Microsoft.Network/dnsZones/example.com/providers/Microsoft.Authorization/permissions"
		if r.URL.Path != wantPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, wantPath)
		}
		if r.URL.Query().Get("api-version") != "2022-04-01" {
			t.Errorf("api-version = %q", r.URL.Query().Get("api-version"))
		}
		fmt.Fprintf(w, `{"value":[{"actions":["Microsoft.Network/dnsZones/A/*"],"notActions":["*/delete"]}],"nextLink":%q}`, server.URL+"/next")
	}))
	defer server.Close()

	checker := &PermissionChecker{credential: testTokenCredential{}, httpClient: server.Client(), endpoint: server.URL}
	result, err := checker.Check(context.Background(), model.AzureConfig{
		Enabled:        true,
		ManageDNS:      true,
		SubscriptionID: "sub-id",
		DNSZones:       []model.AzureDNSZoneConfig{{Name: "example.com", ResourceGroup: "dns-rg"}},
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	if len(result.DNS.Targets) != 1 || !result.DNS.Targets[0].Granted || len(result.DNS.Targets[0].MissingActions) != 0 {
		t.Fatalf("DNS permission result = %#v", result.DNS.Targets)
	}
}

func TestPermissionCheckerUsesNSGResourceGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/subscriptions/sub-id/resourceGroups/Personal_Tools/providers/Microsoft.Network/networkSecurityGroups/farm-gateway-nsg/providers/Microsoft.Authorization/permissions"
		if r.URL.Path != wantPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, wantPath)
		}
		io.WriteString(w, `{"value":[{"actions":["Microsoft.Network/networkSecurityGroups/securityRules/*"]}]}`)
	}))
	defer server.Close()

	checker := &PermissionChecker{credential: testTokenCredential{}, httpClient: server.Client(), endpoint: server.URL}
	result, err := checker.Check(context.Background(), model.AzureConfig{
		Enabled:                           true,
		ManageNSG:                         true,
		SubscriptionID:                    "sub-id",
		ResourceGroup:                     "thingsbud.com",
		NetworkSecurityGroupResourceGroup: "Personal_Tools",
		NetworkSecurityGroupName:          "farm-gateway-nsg",
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(result.Network.Targets) != 1 || !result.Network.Targets[0].Granted {
		t.Fatalf("network permission result = %#v", result.Network.Targets)
	}
}
