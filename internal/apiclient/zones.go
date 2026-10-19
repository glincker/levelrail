package apiclient

import (
	"context"
	"net/http"
)

// ZoneSummary is one failure domain and what it carries.
type ZoneSummary struct {
	Zone      string   `json:"zone"`
	Region    string   `json:"region,omitempty"`
	Providers []string `json:"providers,omitempty"`
	NodeIDs   []string `json:"node_ids"`
	Online    int      `json:"online_nodes"`
	Workloads int      `json:"workloads"`
}

// ZoneNode is one node with its failure-domain labels.
type ZoneNode struct {
	NodeID    string `json:"node_id"`
	Name      string `json:"name"`
	Zone      string `json:"zone,omitempty"`
	Region    string `json:"region,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Online    bool   `json:"online"`
	Workloads int    `json:"workloads"`
}

// Zones is the body of GET /api/v1/nodes/zones.
type Zones struct {
	Zones   []ZoneSummary `json:"zones"`
	Nodes   []ZoneNode    `json:"nodes"`
	Labeled bool          `json:"labeled"`
}

// GetZones calls GET /api/v1/nodes/zones.
func (c *Client) GetZones(ctx context.Context) (Zones, error) {
	var out Zones
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/zones", nil, &out)
	return out, err
}
