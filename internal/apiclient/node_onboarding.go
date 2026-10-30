package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// NodePreflightCheck mirrors internal/nodepreflight.Check.
type NodePreflightCheck struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	Fix        string `json:"fix,omitempty"`
	FixCommand string `json:"fix_command,omitempty"`
}

// NodePreflightSummary mirrors internal/nodepreflight.Summary.
type NodePreflightSummary struct {
	Pass    int  `json:"pass"`
	Warn    int  `json:"warn"`
	Fail    int  `json:"fail"`
	Unknown int  `json:"unknown"`
	Ready   bool `json:"ready"`
}

// NodeDetectedDomain is a provider, region and zone, detected or stored.
type NodeDetectedDomain struct {
	Provider         string `json:"provider,omitempty"`
	Region           string `json:"region,omitempty"`
	Zone             string `json:"zone,omitempty"`
	InstanceID       string `json:"instance_id,omitempty"`
	Source           string `json:"source,omitempty"`
	MetadataDisabled bool   `json:"metadata_disabled,omitempty"`
}

// NodeOnboarding mirrors internal/api's nodeOnboardingResponse.
type NodeOnboarding struct {
	NodeID     string               `json:"node_id"`
	Stage      string               `json:"stage"`
	Reported   bool                 `json:"reported"`
	Stale      bool                 `json:"stale"`
	ReportedAt *time.Time           `json:"reported_at,omitempty"`
	Detected   *NodeDetectedDomain  `json:"detected,omitempty"`
	Domain     NodeDetectedDomain   `json:"domain"`
	Checks     []NodePreflightCheck `json:"checks"`
	Summary    NodePreflightSummary `json:"summary"`
}

// Done reports whether the node reported facts, so the preflight is final.
func (o NodeOnboarding) Done() bool { return o.Reported && !o.Stale }

// GetNodeOnboarding calls GET /api/v1/nodes/{id}/onboarding.
func (c *Client) GetNodeOnboarding(ctx context.Context, id string) (NodeOnboarding, error) {
	var out NodeOnboarding
	err := c.do(ctx, http.MethodGet, nodesCollectionPath()+"/"+url.PathEscape(id)+"/onboarding", nil, &out)
	return out, err
}

// NodeDomainGroup mirrors internal/placement.DomainGroup.
type NodeDomainGroup struct {
	Name          string   `json:"name"`
	Region        string   `json:"region,omitempty"`
	Providers     []string `json:"providers,omitempty"`
	NodeIDs       []string `json:"node_ids"`
	Nodes         int      `json:"nodes"`
	Online        int      `json:"online_nodes"`
	Schedulable   int      `json:"schedulable_nodes"`
	Workloads     int      `json:"workloads"`
	CPUMilli      int64    `json:"cpu_milli"`
	MemoryBytes   int64    `json:"memory_bytes"`
	DiskFreeBytes int64    `json:"disk_free_bytes"`
	Unverified    int      `json:"unverified_nodes"`
}

// NodeDomainRisk mirrors internal/placement.DomainRisk.
type NodeDomainRisk struct {
	Kind         string   `json:"kind"`
	Level        string   `json:"level"`
	Name         string   `json:"name"`
	NodeID       string   `json:"node_id"`
	Replicas     int      `json:"replicas"`
	Zone         string   `json:"zone,omitempty"`
	Region       string   `json:"region,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Alternatives []string `json:"alternatives"`
	Message      string   `json:"message"`
}

// NodeDomains mirrors internal/api's nodeDomainsResponse.
type NodeDomains struct {
	Providers []NodeDomainGroup `json:"providers"`
	Regions   []NodeDomainGroup `json:"regions"`
	Zones     []NodeDomainGroup `json:"zones"`
	Unlabeled int               `json:"unlabeled_nodes"`
	Total     int               `json:"total_nodes"`
	Risks     []NodeDomainRisk  `json:"risks"`
}

// GetNodeDomains calls GET /api/v1/nodes/domains.
func (c *Client) GetNodeDomains(ctx context.Context) (NodeDomains, error) {
	var out NodeDomains
	err := c.do(ctx, http.MethodGet, nodesCollectionPath()+"/domains", nil, &out)
	return out, err
}
