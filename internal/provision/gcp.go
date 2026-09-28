package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	gcpComputeBaseURL = "https://compute.googleapis.com/compute/v1/projects/"
	gcpScope          = "https://www.googleapis.com/auth/compute"
	gcpImageProject   = "ubuntu-os-cloud"
	gcpImageFamily    = "ubuntu-2404-lts-amd64"
	gcpBootDiskGB     = "20"

	// gcpManagedLabelKey mirrors azureManagedTagKey's own reasoning: a
	// fixed, brand-independent label, not brand.ShortName.
	gcpManagedLabelKey = "platform-managed"
)

// gcpServiceAccountKey is the subset of a GCP service account JSON key
// file this package reads directly; the rest of the file is handed to
// google.JWTConfigFromJSON unparsed.
type gcpServiceAccountKey struct {
	ProjectID string `json:"project_id"`
}

func parseGCPProjectID(credentialJSON string) (string, error) {
	var sa gcpServiceAccountKey
	if err := json.Unmarshal([]byte(credentialJSON), &sa); err != nil {
		return "", fmt.Errorf("provision: gcp credential must be a service account JSON key: %w", err)
	}
	if sa.ProjectID == "" {
		return "", fmt.Errorf("provision: gcp credential missing project_id")
	}
	return sa.ProjectID, nil
}

// GCP implements Provisioner against the Compute Engine REST API
// (https://cloud.google.com/compute/docs/reference/rest/v1), a service
// account JSON key exchanged for an OAuth2 access token via JWT bearer
// auth, no SDK (golang.org/x/oauth2/google, not google.golang.org/api).
type GCP struct {
	client    *httpClient
	projectID string
}

// NewGCP returns a GCP provisioner authenticating with credentialJSON, a
// service account key's raw JSON content. The project ID is read from
// the key itself, not a separate field.
func NewGCP(credentialJSON string) (*GCP, error) {
	projectID, err := parseGCPProjectID(credentialJSON)
	if err != nil {
		return nil, err
	}
	jwtConfig, err := google.JWTConfigFromJSON([]byte(credentialJSON), gcpScope)
	if err != nil {
		return nil, fmt.Errorf("provision: gcp credential: %w", err)
	}
	// context.Background: this token source is held for the provisioner's
	// lifetime and refreshes itself on its own schedule, not tied to any
	// single caller's request context.
	return newGCP(projectID, jwtConfig.TokenSource(context.Background()), gcpComputeBaseURL+projectID), nil
}

// newGCP is the seam gcp_test.go uses to point at a fake token source
// and a fake Compute API instead of Google's real ones.
func newGCP(projectID string, ts oauth2.TokenSource, base string) *GCP {
	return &GCP{
		client: newHTTPClientWithTokenFunc(base, func(context.Context) (string, error) {
			tok, err := ts.Token()
			if err != nil {
				return "", err
			}
			return tok.AccessToken, nil
		}),
		projectID: projectID,
	}
}

type gcpZonesResponse struct {
	Items []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"items"`
	NextPageToken string `json:"nextPageToken"`
}

// ListRegions returns GCP zones (e.g. us-central1-a), not regions:
// Compute Engine places an instance in a zone, and CreateServer/
// GetServer/DeleteServer all need that zone, so this interface's
// "region" maps to a GCE zone for this provider.
func (g *GCP) ListRegions(ctx context.Context) ([]Region, error) {
	var zones []Region
	path := "/zones"
	for path != "" {
		var out gcpZonesResponse
		if err := g.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, fmt.Errorf("provision: gcp list zones: %w", err)
		}
		for _, z := range out.Items {
			if z.Status != "UP" {
				continue
			}
			zones = append(zones, Region{ID: z.Name, Name: z.Name})
		}
		if out.NextPageToken == "" {
			return zones, nil
		}
		path = "/zones?pageToken=" + url.QueryEscape(out.NextPageToken)
	}
	return zones, nil
}

type gcpMachineTypesResponse struct {
	Items []struct {
		Name      string `json:"name"`
		GuestCPUs int    `json:"guestCpus"`
		MemoryMB  int    `json:"memoryMb"`
		// Deprecated is non-nil once the machine type has any deprecation
		// state set (Google's own "deprecated"/"obsolete"/"deleted").
		Deprecated *struct {
			State string `json:"state"`
		} `json:"deprecated"`
	} `json:"items"`
	NextPageToken string `json:"nextPageToken"`
}

// ListSizes calls GET .../zones/{zone}/machineTypes. Disk size is a
// separate resource in GCE, not part of a machine type, so every
// returned Size has Disk 0; CreateServer picks gcpBootDiskGB itself.
// GCP's own billing catalog is a separate API this package doesn't call,
// so PriceMonthly is always empty here too.
func (g *GCP) ListSizes(ctx context.Context, zone string) ([]Size, error) {
	if zone == "" {
		return nil, fmt.Errorf("provision: gcp list sizes: zone is required")
	}
	var sizes []Size
	path := "/zones/" + zone + "/machineTypes"
	for path != "" {
		var out gcpMachineTypesResponse
		if err := g.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, fmt.Errorf("provision: gcp list sizes: %w", err)
		}
		for _, m := range out.Items {
			if m.Deprecated != nil && m.Deprecated.State != "" {
				continue
			}
			sizes = append(sizes, Size{ID: m.Name, Name: m.Name, VCPUs: m.GuestCPUs, Memory: m.MemoryMB})
		}
		if out.NextPageToken == "" {
			return sizes, nil
		}
		path = "/zones/" + zone + "/machineTypes?pageToken=" + url.QueryEscape(out.NextPageToken)
	}
	return sizes, nil
}

type gcpOperationResponse struct {
	Error *struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"error"`
}

// CreateServer calls POST .../zones/{zone}/instances with an ephemeral
// external IP (no reserved address to clean up later) and an
// auto-deleting boot disk, so DeleteServer needs only the one instance
// delete call. The insert call is asynchronous at the operation level,
// but the instance resource itself exists as soon as this call returns,
// in PROVISIONING state; its IP isn't assigned yet, so ipAddr is always
// empty here, matching CreateOpts' own doc comment for a provider that
// assigns one later. serverID is "{zone}/{name}": unlike Azure's fixed
// per-account resource group, GCP's zone varies per server and every
// later call needs it, so it has to travel in the id.
func (g *GCP) CreateServer(ctx context.Context, opts CreateOpts) (serverID, ipAddr string, err error) {
	if opts.Region == "" || opts.Size == "" || opts.Name == "" {
		return "", "", fmt.Errorf("provision: gcp create server: zone, size and name are required")
	}
	body := map[string]any{
		"name":        opts.Name,
		"machineType": fmt.Sprintf("zones/%s/machineTypes/%s", opts.Region, opts.Size),
		"disks": []map[string]any{
			{
				"boot": true, "autoDelete": true,
				"initializeParams": map[string]any{
					"sourceImage": fmt.Sprintf("projects/%s/global/images/family/%s", gcpImageProject, gcpImageFamily),
					"diskSizeGb":  gcpBootDiskGB,
				},
			},
		},
		"networkInterfaces": []map[string]any{
			{
				"network":       "global/networks/default",
				"accessConfigs": []map[string]any{{"type": "ONE_TO_ONE_NAT", "name": "External NAT"}},
			},
		},
		"metadata": map[string]any{
			"items": []map[string]string{{"key": "user-data", "value": opts.UserData}},
		},
		"labels": map[string]string{gcpManagedLabelKey: "true"},
	}
	var out gcpOperationResponse
	path := "/zones/" + opts.Region + "/instances"
	if err := g.client.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return "", "", fmt.Errorf("provision: gcp create server: %w", err)
	}
	if out.Error != nil && len(out.Error.Errors) > 0 {
		return "", "", fmt.Errorf("provision: gcp create server: %s", out.Error.Errors[0].Message)
	}
	return opts.Region + "/" + opts.Name, "", nil
}

func splitGCPServerID(id string) (zone, name string, err error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("provision: gcp: malformed server id %q, want zone/name", id)
	}
	return parts[0], parts[1], nil
}

type gcpInstanceResponse struct {
	Status            string `json:"status"`
	NetworkInterfaces []struct {
		AccessConfigs []struct {
			NatIP string `json:"natIP"`
		} `json:"accessConfigs"`
	} `json:"networkInterfaces"`
}

// GetServer calls GET .../zones/{zone}/instances/{name}, id being the
// "{zone}/{name}" CreateServer returned.
func (g *GCP) GetServer(ctx context.Context, id string) (ServerStatus, string, error) {
	zone, name, err := splitGCPServerID(id)
	if err != nil {
		return "", "", err
	}
	var out gcpInstanceResponse
	path := fmt.Sprintf("/zones/%s/instances/%s", zone, name)
	if err := g.client.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", "", fmt.Errorf("provision: gcp get server %q: %w", id, err)
	}
	var ip string
	if len(out.NetworkInterfaces) > 0 && len(out.NetworkInterfaces[0].AccessConfigs) > 0 {
		ip = out.NetworkInterfaces[0].AccessConfigs[0].NatIP
	}
	return gcpStatus(out.Status), ip, nil
}

func gcpStatus(s string) ServerStatus {
	switch s {
	case "RUNNING":
		return ServerStatusRunning
	case "TERMINATED", "STOPPING", "SUSPENDED", "SUSPENDING":
		return ServerStatusError
	default:
		// PROVISIONING, STAGING, REPAIRING
		return ServerStatusPending
	}
}

// DeleteServer calls DELETE .../zones/{zone}/instances/{name}. The boot
// disk (autoDelete: true) and the ephemeral external IP go with it; no
// separate cleanup call is needed.
func (g *GCP) DeleteServer(ctx context.Context, id string) error {
	zone, name, err := splitGCPServerID(id)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/zones/%s/instances/%s", zone, name)
	if err := g.client.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("provision: gcp delete server %q: %w", id, err)
	}
	return nil
}
