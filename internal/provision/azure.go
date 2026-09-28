package provision

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	azureManagementBaseURL       = "https://management.azure.com"
	azureAPIVersionSubscriptions = "2022-12-01"
	azureAPIVersionResources     = "2021-04-01"
	azureAPIVersionNetwork       = "2023-09-01"
	azureAPIVersionCompute       = "2024-07-01"

	azureImagePublisher = "Canonical"
	azureImageOffer     = "ubuntu-24_04-lts"
	azureImageSKU       = "server"
	azureAdminUsername  = "leveladmin"

	// azureVNetPrefix names the one VNet/subnet this provisioner shares
	// across every server it creates in a given region (created lazily,
	// never deleted by DeleteServer): a per-server VNet would leak on
	// delete, since only the NIC, public IP and OS disk cascade-delete
	// with the VM (see DeleteServer).
	azureVNetPrefix = "node-provisioning-vnet-"
	azureSubnetName = "default"
	azureVNetCIDR   = "10.60.0.0/16"
	azureSubnetCIDR = "10.60.0.0/24"

	// azureManagedTagKey mirrors internal/spec.ReservedLabelPrefix's own
	// reasoning: a fixed, brand-independent tag, not brand.ShortName,
	// since brand config is runtime-configurable and resource cleanup
	// must not depend on it.
	azureManagedTagKey = "platform-managed"
)

// azureCredential is the JSON object shape the node-provider credential
// store's single "token" field holds for Azure: a service principal
// (Azure AD app registration) with Contributor access scoped to
// ResourceGroup, authenticating either with a client secret (OAuth2
// client-credentials flow, the default) or, when FederatedTokenFile is
// set instead, workload identity federation (see azure_federation.go).
type azureCredential struct {
	TenantID       string `json:"tenant_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret,omitempty"`
	SubscriptionID string `json:"subscription_id"`
	ResourceGroup  string `json:"resource_group"`
	// FederatedTokenFile, when set, switches auth from ClientSecret to
	// workload identity federation (OIDC): the path to a file holding a
	// JWT signed by an external OIDC issuer this app registration trusts
	// via a federated credential configured on the Azure side. Mutually
	// exclusive with ClientSecret; ClientSecret takes precedence if both
	// are set, since a static secret is unambiguous where a stale or
	// misconfigured federation setup is not.
	FederatedTokenFile string `json:"federated_token_file,omitempty"`
}

func parseAzureCredential(raw string) (azureCredential, error) {
	var cred azureCredential
	if err := json.Unmarshal([]byte(raw), &cred); err != nil {
		return azureCredential{}, fmt.Errorf("provision: azure credential must be a JSON object with tenant_id, client_id, client_secret (or federated_token_file), subscription_id, resource_group: %w", err)
	}
	switch {
	case cred.TenantID == "":
		return azureCredential{}, fmt.Errorf("provision: azure credential missing tenant_id")
	case cred.ClientID == "":
		return azureCredential{}, fmt.Errorf("provision: azure credential missing client_id")
	case cred.ClientSecret == "" && cred.FederatedTokenFile == "":
		return azureCredential{}, fmt.Errorf("provision: azure credential missing client_secret (or federated_token_file for workload identity federation)")
	case cred.SubscriptionID == "":
		return azureCredential{}, fmt.Errorf("provision: azure credential missing subscription_id")
	case cred.ResourceGroup == "":
		return azureCredential{}, fmt.Errorf("provision: azure credential missing resource_group")
	}
	return cred, nil
}

// Azure implements Provisioner against the Azure Resource Manager REST
// API (https://learn.microsoft.com/en-us/rest/api/compute/), OAuth2
// client-credentials auth, no SDK. Unlike Hetzner/DigitalOcean's static
// bearer token, every request needs a fresh access token: httpClient's
// tokenFunc seam (httpclient.go) resolves one from ts on each call.
type Azure struct {
	client         *httpClient
	subscriptionID string
	resourceGroup  string
}

// NewAzure returns an Azure provisioner authenticating with credentialJSON
// (see azureCredential): a client secret by default, or workload identity
// federation when credentialJSON sets federated_token_file instead.
func NewAzure(credentialJSON string) (*Azure, error) {
	cred, err := parseAzureCredential(credentialJSON)
	if err != nil {
		return nil, err
	}
	var ts oauth2.TokenSource
	if cred.ClientSecret == "" && cred.FederatedTokenFile != "" {
		// oauth2.ReuseTokenSource caches the AAD access token until it
		// expires, so azureFederatedTokenSource's own file read only
		// happens on that same cadence, not on every provider API call.
		ts = oauth2.ReuseTokenSource(nil, newAzureFederatedTokenSource(cred))
	} else {
		cfg := clientcredentials.Config{
			ClientID:     cred.ClientID,
			ClientSecret: cred.ClientSecret,
			TokenURL:     "https://login.microsoftonline.com/" + cred.TenantID + "/oauth2/v2.0/token",
			Scopes:       []string{"https://management.azure.com/.default"},
		}
		// context.Background: this token source is held for the
		// provisioner's lifetime and refreshes itself on its own
		// schedule, not tied to any single caller's request context.
		ts = cfg.TokenSource(context.Background())
	}
	return newAzure(cred, ts, azureManagementBaseURL), nil
}

// newAzure is the seam azure_test.go uses to point at a fake token
// source and a fake management API instead of Microsoft's real ones.
func newAzure(cred azureCredential, ts oauth2.TokenSource, base string) *Azure {
	return &Azure{
		client: newHTTPClientWithTokenFunc(base, func(context.Context) (string, error) {
			tok, err := ts.Token()
			if err != nil {
				return "", err
			}
			return tok.AccessToken, nil
		}),
		subscriptionID: cred.SubscriptionID,
		resourceGroup:  cred.ResourceGroup,
	}
}

func (a *Azure) rgPath(suffix string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s%s", a.subscriptionID, a.resourceGroup, suffix)
}

type azureLocationsResponse struct {
	Value []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Metadata    struct {
			RegionType string `json:"regionType"`
		} `json:"metadata"`
	} `json:"value"`
}

// ListRegions calls GET /subscriptions/{sub}/locations, keeping only
// physical regions (Azure also lists "Logical" entries like paired DR
// regions that aren't valid CreateServer targets).
func (a *Azure) ListRegions(ctx context.Context) ([]Region, error) {
	var out azureLocationsResponse
	path := fmt.Sprintf("/subscriptions/%s/locations?api-version=%s", a.subscriptionID, azureAPIVersionSubscriptions)
	if err := a.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("provision: azure list regions: %w", err)
	}
	var regions []Region
	for _, l := range out.Value {
		if l.Metadata.RegionType != "" && l.Metadata.RegionType != "Physical" {
			continue
		}
		regions = append(regions, Region{ID: l.Name, Name: l.DisplayName})
	}
	return regions, nil
}

type azureVMSizesResponse struct {
	Value []struct {
		Name           string `json:"name"`
		NumberOfCores  int    `json:"numberOfCores"`
		MemoryInMB     int    `json:"memoryInMB"`
		OSDiskSizeInMB int    `json:"osDiskSizeInMB"`
	} `json:"value"`
}

// ListSizes calls GET .../locations/{region}/vmSizes. Azure's retail
// pricing isn't part of this API, so every returned Size has an empty
// PriceMonthly, the same "not returned" case DigitalOcean/Hetzner's own
// Size doc comment already covers.
func (a *Azure) ListSizes(ctx context.Context, region string) ([]Size, error) {
	if region == "" {
		return nil, fmt.Errorf("provision: azure list sizes: region is required")
	}
	var out azureVMSizesResponse
	path := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Compute/locations/%s/vmSizes?api-version=%s", a.subscriptionID, region, azureAPIVersionCompute)
	if err := a.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("provision: azure list sizes: %w", err)
	}
	sizes := make([]Size, 0, len(out.Value))
	for _, s := range out.Value {
		sizes = append(sizes, Size{
			ID: s.Name, Name: s.Name, VCPUs: s.NumberOfCores, Memory: s.MemoryInMB, Disk: s.OSDiskSizeInMB / 1024,
		})
	}
	return sizes, nil
}

// CreateServer provisions a resource group (idempotent, never deleted),
// a shared per-region VNet/subnet (idempotent, never deleted), then a
// public IP, NIC and virtual machine unique to this server. No network
// security group is created: a Standard SKU public IP is closed to
// inbound traffic by default unless an NSG explicitly allows it, so this
// matches the "no inbound ports" default the other providers get from
// their own platform defaults.
func (a *Azure) CreateServer(ctx context.Context, opts CreateOpts) (serverID, ipAddr string, err error) {
	if opts.Region == "" || opts.Size == "" || opts.Name == "" {
		return "", "", fmt.Errorf("provision: azure create server: region, size and name are required")
	}
	if err := a.ensureResourceGroup(ctx, opts.Region); err != nil {
		return "", "", err
	}
	subnetID, err := a.ensureNetwork(ctx, opts.Region)
	if err != nil {
		return "", "", err
	}
	pipID, pipAddr, err := a.createPublicIP(ctx, opts.Region, opts.Name)
	if err != nil {
		return "", "", err
	}
	nicID, err := a.createNIC(ctx, opts.Region, opts.Name, subnetID, pipID)
	if err != nil {
		return "", "", err
	}
	if err := a.createVM(ctx, opts, nicID); err != nil {
		return "", "", err
	}
	return opts.Name, pipAddr, nil
}

func (a *Azure) ensureResourceGroup(ctx context.Context, location string) error {
	body := map[string]any{"location": location}
	path := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s?api-version=%s", a.subscriptionID, a.resourceGroup, azureAPIVersionResources)
	if err := a.client.do(ctx, http.MethodPut, path, body, nil); err != nil {
		return fmt.Errorf("provision: azure ensure resource group: %w", err)
	}
	return nil
}

func (a *Azure) ensureNetwork(ctx context.Context, location string) (string, error) {
	vnetName := azureVNetPrefix + location
	body := map[string]any{
		"location": location,
		"properties": map[string]any{
			"addressSpace": map[string]any{"addressPrefixes": []string{azureVNetCIDR}},
			"subnets": []map[string]any{
				{"name": azureSubnetName, "properties": map[string]any{"addressPrefix": azureSubnetCIDR}},
			},
		},
	}
	var out struct {
		Properties struct {
			Subnets []struct {
				ID string `json:"id"`
			} `json:"subnets"`
		} `json:"properties"`
	}
	path := a.rgPath(fmt.Sprintf("/providers/Microsoft.Network/virtualNetworks/%s?api-version=%s", vnetName, azureAPIVersionNetwork))
	if err := a.client.do(ctx, http.MethodPut, path, body, &out); err != nil {
		return "", fmt.Errorf("provision: azure ensure network: %w", err)
	}
	if len(out.Properties.Subnets) == 0 {
		return "", fmt.Errorf("provision: azure ensure network: response had no subnet")
	}
	return out.Properties.Subnets[0].ID, nil
}

func (a *Azure) createPublicIP(ctx context.Context, location, name string) (id, address string, err error) {
	body := map[string]any{
		"location": location,
		"sku":      map[string]any{"name": "Standard"},
		"properties": map[string]any{
			"publicIPAllocationMethod": "Static",
			// Cascades this public IP's deletion from the NIC it ends up
			// attached to, itself cascaded from the VM (see DeleteServer).
			"deleteOption": "Delete",
		},
		"tags": map[string]string{azureManagedTagKey: "true"},
	}
	var out struct {
		ID         string `json:"id"`
		Properties struct {
			IPAddress string `json:"ipAddress"`
		} `json:"properties"`
	}
	path := a.rgPath(fmt.Sprintf("/providers/Microsoft.Network/publicIPAddresses/%s-pip?api-version=%s", name, azureAPIVersionNetwork))
	if err := a.client.do(ctx, http.MethodPut, path, body, &out); err != nil {
		return "", "", fmt.Errorf("provision: azure create public ip: %w", err)
	}
	return out.ID, out.Properties.IPAddress, nil
}

func (a *Azure) createNIC(ctx context.Context, location, name, subnetID, publicIPID string) (string, error) {
	body := map[string]any{
		"location": location,
		"properties": map[string]any{
			"ipConfigurations": []map[string]any{
				{
					"name": "ipconfig1",
					"properties": map[string]any{
						"subnet":                    map[string]any{"id": subnetID},
						"publicIPAddress":           map[string]any{"id": publicIPID},
						"privateIPAllocationMethod": "Dynamic",
					},
				},
			},
		},
		"tags": map[string]string{azureManagedTagKey: "true"},
	}
	var out struct {
		ID string `json:"id"`
	}
	path := a.rgPath(fmt.Sprintf("/providers/Microsoft.Network/networkInterfaces/%s-nic?api-version=%s", name, azureAPIVersionNetwork))
	if err := a.client.do(ctx, http.MethodPut, path, body, &out); err != nil {
		return "", fmt.Errorf("provision: azure create network interface: %w", err)
	}
	return out.ID, nil
}

func (a *Azure) createVM(ctx context.Context, opts CreateOpts, nicID string) error {
	password, err := randomAzureAdminPassword()
	if err != nil {
		return fmt.Errorf("provision: azure create server: %w", err)
	}
	body := map[string]any{
		"location": opts.Region,
		"properties": map[string]any{
			"hardwareProfile": map[string]any{"vmSize": opts.Size},
			"storageProfile": map[string]any{
				"imageReference": map[string]any{
					"publisher": azureImagePublisher, "offer": azureImageOffer, "sku": azureImageSKU, "version": "latest",
				},
				"osDisk": map[string]any{"createOption": "FromImage", "deleteOption": "Delete"},
			},
			"osProfile": map[string]any{
				"computerName":  opts.Name,
				"adminUsername": azureAdminUsername,
				"adminPassword": password,
				"customData":    base64.StdEncoding.EncodeToString([]byte(opts.UserData)),
			},
			"networkProfile": map[string]any{
				"networkInterfaces": []map[string]any{
					{"id": nicID, "properties": map[string]any{"primary": true, "deleteOption": "Delete"}},
				},
			},
		},
		"tags": map[string]string{azureManagedTagKey: "true"},
	}
	path := a.rgPath(fmt.Sprintf("/providers/Microsoft.Compute/virtualMachines/%s?api-version=%s", opts.Name, azureAPIVersionCompute))
	if err := a.client.do(ctx, http.MethodPut, path, body, nil); err != nil {
		return fmt.Errorf("provision: azure create virtual machine: %w", err)
	}
	return nil
}

// randomAzureAdminPassword satisfies Azure's VM creation API, which
// requires a password when no SSH public key is supplied (CreateOpts has
// none): generated once, sent to the API, never stored or returned. The
// agent enrolls by dialing out to the control plane, never over SSH, so
// no admin actually needs this password; an operator who wants SSH
// access to an Azure-provisioned node must set one up separately.
func randomAzureAdminPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate admin password: %w", err)
	}
	// Azure requires 3 of 4 character classes; this fixed prefix
	// guarantees upper, lower, digit and symbol regardless of what
	// rand.Read draws for the rest.
	return "Az9!" + base64.RawURLEncoding.EncodeToString(buf), nil
}

type azureVMResponse struct {
	Properties struct {
		ProvisioningState string `json:"provisioningState"`
	} `json:"properties"`
}

// GetServer calls GET .../virtualMachines/{id} plus a second call for
// the public IP's address, id being the VM name (resourceGroup and
// subscriptionID are fixed per Azure instance, not part of id).
func (a *Azure) GetServer(ctx context.Context, id string) (ServerStatus, string, error) {
	var out azureVMResponse
	path := a.rgPath(fmt.Sprintf("/providers/Microsoft.Compute/virtualMachines/%s?api-version=%s", id, azureAPIVersionCompute))
	if err := a.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", "", fmt.Errorf("provision: azure get server %q: %w", id, err)
	}
	ipAddr, err := a.getPublicIP(ctx, id)
	if err != nil {
		return "", "", fmt.Errorf("provision: azure get server %q: %w", id, err)
	}
	return azureStatus(out.Properties.ProvisioningState), ipAddr, nil
}

func (a *Azure) getPublicIP(ctx context.Context, name string) (string, error) {
	var out struct {
		Properties struct {
			IPAddress string `json:"ipAddress"`
		} `json:"properties"`
	}
	path := a.rgPath(fmt.Sprintf("/providers/Microsoft.Network/publicIPAddresses/%s-pip?api-version=%s", name, azureAPIVersionNetwork))
	if err := a.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", fmt.Errorf("get public ip: %w", err)
	}
	return out.Properties.IPAddress, nil
}

func azureStatus(s string) ServerStatus {
	switch s {
	case "Succeeded":
		return ServerStatusRunning
	case "Failed":
		return ServerStatusError
	default:
		// Creating, Updating, Deleting, etc.
		return ServerStatusPending
	}
}

// DeleteServer deletes only the virtual machine. Its NIC, public IP and
// OS disk were all created with deleteOption "Delete" (createVM,
// createNIC, createPublicIP), which Azure documents as cascading their
// deletion from this one call; the shared VNet/subnet is left in place
// for other servers in the same region. This cascade was not verified
// against a real Azure subscription, only against fakes (see azure_test.go).
func (a *Azure) DeleteServer(ctx context.Context, id string) error {
	path := a.rgPath(fmt.Sprintf("/providers/Microsoft.Compute/virtualMachines/%s?api-version=%s&forceDeletion=true", id, azureAPIVersionCompute))
	if err := a.client.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("provision: azure delete server %q: %w", id, err)
	}
	return nil
}
