package apiclient

import (
	"context"
	"net/http"
	"time"
)

// NetworkPool is the pool block of GET /api/v1/system/networks.
type NetworkPool struct {
	CIDR     string `json:"cidr"`
	Enabled  bool   `json:"enabled"`
	Capacity int    `json:"capacity"`
	InUse    int    `json:"in_use"`
	Percent  int    `json:"percent"`
}

// SystemNetwork is one product network.
type SystemNetwork struct {
	Name               string    `json:"name"`
	OwnerKind          string    `json:"owner_kind"`
	OwnerName          string    `json:"owner_name"`
	Subnets            []string  `json:"subnets"`
	AttachedContainers int       `json:"attached_containers"`
	Orphaned           bool      `json:"orphaned"`
	Removable          bool      `json:"removable"`
	Reason             string    `json:"reason"`
	Foreign            bool      `json:"foreign_instance"`
	CreatedAt          time.Time `json:"created_at"`
}

// SystemNetworks is the GET /api/v1/system/networks resource.
type SystemNetworks struct {
	Pool        NetworkPool     `json:"pool"`
	Networks    []SystemNetwork `json:"networks"`
	OrphanCount int             `json:"orphan_count"`
}

// SystemNetworksPrune is the POST /api/v1/system/networks/prune resource.
type SystemNetworksPrune struct {
	DryRun     bool     `json:"dry_run"`
	Candidates []string `json:"candidates"`
	Removed    []string `json:"removed"`
}

// GetSystemNetworks calls GET /api/v1/system/networks.
func (c *Client) GetSystemNetworks(ctx context.Context) (SystemNetworks, error) {
	var out SystemNetworks
	err := c.do(ctx, http.MethodGet, "/api/v1/system/networks", nil, &out)
	return out, err
}

// PruneSystemNetworks calls POST /api/v1/system/networks/prune.
func (c *Client) PruneSystemNetworks(ctx context.Context, dryRun bool) (SystemNetworksPrune, error) {
	var out SystemNetworksPrune
	err := c.do(ctx, http.MethodPost, "/api/v1/system/networks/prune", map[string]bool{"dry_run": dryRun}, &out)
	return out, err
}
