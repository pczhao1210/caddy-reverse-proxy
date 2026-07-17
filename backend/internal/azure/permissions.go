package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/aidockerfarm/gateway/internal/model"
)

const azureManagementEndpoint = "https://management.azure.com"

var (
	dnsRequiredActions = []string{
		"Microsoft.Network/dnsZones/A/read",
		"Microsoft.Network/dnsZones/A/write",
		"Microsoft.Network/dnsZones/A/delete",
	}
	networkRequiredActions = []string{
		"Microsoft.Network/networkSecurityGroups/securityRules/read",
		"Microsoft.Network/networkSecurityGroups/securityRules/write",
		"Microsoft.Network/networkSecurityGroups/securityRules/delete",
	}
)

type PermissionChecker struct {
	credential azcore.TokenCredential
	httpClient httpDoer
	endpoint   string
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type PermissionCheckResult struct {
	CheckedAt  time.Time             `json:"checkedAt"`
	Credential string                `json:"credential"`
	DNS        PermissionGroupResult `json:"dns"`
	Network    PermissionGroupResult `json:"network"`
}

type PermissionGroupResult struct {
	Configured bool                     `json:"configured"`
	Targets    []PermissionTargetResult `json:"targets,omitempty"`
}

type PermissionTargetResult struct {
	Name            string   `json:"name"`
	ResourceGroup   string   `json:"resourceGroup"`
	Scope           string   `json:"scope"`
	Granted         bool     `json:"granted"`
	RequiredActions []string `json:"requiredActions"`
	MissingActions  []string `json:"missingActions,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type permissionBlock struct {
	actions    []string
	notActions []string
}

type effectivePermissionsResponse struct {
	Value    []effectivePermission `json:"value"`
	NextLink string                `json:"nextLink"`
}

type effectivePermission struct {
	Actions    []string `json:"actions"`
	NotActions []string `json:"notActions"`
}

type permissionTarget struct {
	name            string
	resourceGroup   string
	provider        string
	resourceType    string
	requiredActions []string
}

func NewPermissionChecker() (*PermissionChecker, error) {
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return &PermissionChecker{
		credential: credential,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   azureManagementEndpoint,
	}, nil
}

func (c *PermissionChecker) Check(ctx context.Context, cfg model.AzureConfig) (PermissionCheckResult, error) {
	result := PermissionCheckResult{
		CheckedAt:  time.Now().UTC(),
		Credential: "DefaultAzureCredential",
		DNS:        PermissionGroupResult{Configured: cfg.Enabled && cfg.ManageDNS},
		Network:    PermissionGroupResult{Configured: cfg.Enabled && cfg.ManageNSG},
	}
	if !result.DNS.Configured && !result.Network.Configured {
		return result, nil
	}

	subscriptionID := strings.TrimSpace(cfg.SubscriptionID)
	if subscriptionID == "" {
		return result, fmt.Errorf("azure subscriptionId is required to check permissions")
	}
	if result.DNS.Configured {
		zones := configuredDNSZones(cfg)
		if len(zones) == 0 {
			return result, fmt.Errorf("at least one DNS zone with a resource group is required to check DNS permissions")
		}
		result.DNS.Targets = make([]PermissionTargetResult, 0, len(zones))
		for _, zone := range zones {
			target := permissionTarget{
				name:            zone.Name,
				resourceGroup:   zone.ResourceGroup,
				provider:        "Microsoft.Network",
				resourceType:    "dnsZones",
				requiredActions: dnsRequiredActions,
			}
			result.DNS.Targets = append(result.DNS.Targets, c.checkPermissionTarget(ctx, subscriptionID, target))
		}
	}

	if result.Network.Configured {
		resourceGroup := nsgResourceGroup(cfg)
		name := strings.TrimSpace(cfg.NetworkSecurityGroupName)
		if resourceGroup == "" || name == "" {
			return result, fmt.Errorf("Azure networkSecurityGroupResourceGroup and networkSecurityGroupName are required to check network permissions")
		}
		target := permissionTarget{
			name:            name,
			resourceGroup:   resourceGroup,
			provider:        "Microsoft.Network",
			resourceType:    "networkSecurityGroups",
			requiredActions: networkRequiredActions,
		}
		result.Network.Targets = []PermissionTargetResult{c.checkPermissionTarget(ctx, subscriptionID, target)}
	}

	return result, nil
}

func nsgResourceGroup(cfg model.AzureConfig) string {
	if resourceGroup := strings.TrimSpace(cfg.NetworkSecurityGroupResourceGroup); resourceGroup != "" {
		return resourceGroup
	}
	return strings.TrimSpace(cfg.ResourceGroup)
}

func (c *PermissionChecker) checkPermissionTarget(ctx context.Context, subscriptionID string, target permissionTarget) PermissionTargetResult {
	result := PermissionTargetResult{
		Name:            target.name,
		ResourceGroup:   target.resourceGroup,
		Scope:           azureResourceScope(subscriptionID, target),
		RequiredActions: append([]string(nil), target.requiredActions...),
	}
	permissions, err := c.listPermissions(ctx, subscriptionID, target)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.MissingActions = missingActions(target.requiredActions, permissions)
	result.Granted = len(result.MissingActions) == 0
	return result
}

func (c *PermissionChecker) listPermissions(ctx context.Context, subscriptionID string, target permissionTarget) ([]permissionBlock, error) {
	token, err := c.credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{strings.TrimRight(c.endpoint, "/") + "/.default"}})
	if err != nil {
		return nil, fmt.Errorf("authenticate with DefaultAzureCredential: %w", err)
	}
	nextLink := permissionsURL(c.endpoint, subscriptionID, target)
	var permissions []permissionBlock
	for nextLink != "" {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, nextLink, nil)
		if err != nil {
			return nil, fmt.Errorf("create effective permissions request for %s: %w", target.name, err)
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+token.Token)
		response, err := c.httpClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("get effective permissions for %s: %w", target.name, err)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 32*1024))
			response.Body.Close()
			return nil, fmt.Errorf("get effective permissions for %s: Azure returned %s: %s", target.name, response.Status, strings.TrimSpace(string(body)))
		}
		var page effectivePermissionsResponse
		decodeErr := json.NewDecoder(response.Body).Decode(&page)
		response.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode effective permissions for %s: %w", target.name, decodeErr)
		}
		for _, permission := range page.Value {
			permissions = append(permissions, permissionBlock{actions: permission.Actions, notActions: permission.NotActions})
		}
		nextLink = page.NextLink
	}
	return permissions, nil
}

func permissionsURL(endpoint, subscriptionID string, target permissionTarget) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourceGroups/%s/providers/%s/%s/%s/providers/Microsoft.Authorization/permissions?api-version=2022-04-01",
		strings.TrimRight(endpoint, "/"),
		url.PathEscape(subscriptionID),
		url.PathEscape(target.resourceGroup),
		url.PathEscape(target.provider),
		url.PathEscape(target.resourceType),
		url.PathEscape(target.name),
	)
}

func missingActions(required []string, permissions []permissionBlock) []string {
	var missing []string
	for _, action := range required {
		if !actionAllowed(action, permissions) {
			missing = append(missing, action)
		}
	}
	return missing
}

func actionAllowed(action string, permissions []permissionBlock) bool {
	for _, permission := range permissions {
		if matchesAnyAction(permission.actions, action) && !matchesAnyAction(permission.notActions, action) {
			return true
		}
	}
	return false
}

func matchesAnyAction(patterns []string, action string) bool {
	for _, pattern := range patterns {
		if azureActionMatches(pattern, action) {
			return true
		}
	}
	return false
}

func azureActionMatches(pattern, action string) bool {
	pattern = regexp.QuoteMeta(strings.ToLower(strings.TrimSpace(pattern)))
	pattern = strings.ReplaceAll(pattern, `\*`, `.*`)
	matched, _ := regexp.MatchString("^"+pattern+"$", strings.ToLower(strings.TrimSpace(action)))
	return matched
}

func azureResourceScope(subscriptionID string, target permissionTarget) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/%s/%s/%s", subscriptionID, target.resourceGroup, target.provider, target.resourceType, target.name)
}
