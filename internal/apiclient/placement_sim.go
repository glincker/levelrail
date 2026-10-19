package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// SimulatedPlacement is one workload in a placement or failure simulation.
type SimulatedPlacement struct {
	Name     string `json:"name"`
	From     string `json:"from_node_id,omitempty"`
	NodeID   string `json:"node_id,omitempty"`
	NodeName string `json:"node_name,omitempty"`
	Zone     string `json:"zone,omitempty"`
	Why      string `json:"why,omitempty"`
	Unplaced string `json:"unplaced,omitempty"`
}

// PlacementSimulation is the body of GET /api/v1/nodes/placement-simulation.
type PlacementSimulation struct {
	Request struct {
		CPUMilli  int64  `json:"cpu_milli"`
		MemoryMiB int64  `json:"memory_mib"`
		Count     int    `json:"count"`
		Selector  string `json:"selector,omitempty"`
	} `json:"request"`
	Placed   []SimulatedPlacement `json:"placed"`
	Unplaced int                  `json:"unplaced"`
	Zones    []ZoneSummary        `json:"zones"`
}

// FailureSimulation is the body of GET /api/v1/nodes/failure-simulation.
type FailureSimulation struct {
	Zone     string               `json:"zone,omitempty"`
	Node     string               `json:"node,omitempty"`
	Lost     []string             `json:"lost_node_ids"`
	Count    int                  `json:"displaced"`
	Placed   []SimulatedPlacement `json:"placed"`
	Unplaced int                  `json:"unplaced"`
	Zones    []ZoneSummary        `json:"zones"`
}

// SimulatePlacement asks where count apps asking for cpuMilli and memoryMiB would land.
func (c *Client) SimulatePlacement(ctx context.Context, cpuMilli, memoryMiB int64, count int, selector string) (PlacementSimulation, error) {
	q := url.Values{}
	q.Set("cpu_milli", strconv.FormatInt(cpuMilli, 10))
	q.Set("memory_mib", strconv.FormatInt(memoryMiB, 10))
	q.Set("count", strconv.Itoa(count))
	if selector != "" {
		q.Set("selector", selector)
	}
	var out PlacementSimulation
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/placement-simulation?"+q.Encode(), nil, &out)
	return out, err
}

// SimulateFailure asks what would happen if a zone or one node were lost.
func (c *Client) SimulateFailure(ctx context.Context, zone, node string) (FailureSimulation, error) {
	q := url.Values{}
	if zone != "" {
		q.Set("zone", zone)
	}
	if node != "" {
		q.Set("node", node)
	}
	var out FailureSimulation
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/failure-simulation?"+q.Encode(), nil, &out)
	return out, err
}
