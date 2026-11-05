package apiclient

import (
	"context"
	"net/http"
)

// ReachabilityProbe is one local dial of a port.
type ReachabilityProbe struct {
	Address   string `json:"address"`
	Listening bool   `json:"listening"`
}

// ReachabilityPort is the result for one listening port.
type ReachabilityPort struct {
	Name         string              `json:"name"`
	Port         int                 `json:"port"`
	LoopbackOnly bool                `json:"loopback_only,omitempty"`
	Listening    bool                `json:"listening"`
	Probes       []ReachabilityProbe `json:"probes"`
	External     string              `json:"external"`
	Advice       string              `json:"advice,omitempty"`
}

// Reachability is the control plane's port reachability report.
type Reachability struct {
	Status          string             `json:"status"`
	Message         string             `json:"message"`
	Ports           []ReachabilityPort `json:"ports"`
	DialbackEnabled bool               `json:"dialback_enabled"`
	DialbackError   string             `json:"dialback_error,omitempty"`
	Firewall        string             `json:"firewall,omitempty"`
}

// GetReachability reads GET /api/v1/doctor/reachability.
func (c *Client) GetReachability(ctx context.Context) (Reachability, error) {
	var out Reachability
	err := c.do(ctx, http.MethodGet, "/api/v1/doctor/reachability", nil, &out)
	return out, err
}
