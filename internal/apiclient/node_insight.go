package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// NodeDoctor mirrors internal/api's nodeDoctorResponse.
type NodeDoctor struct {
	NodeID   string               `json:"node_id"`
	Reported bool                 `json:"reported"`
	Checks   []NodePreflightCheck `json:"checks"`
	Summary  NodePreflightSummary `json:"summary"`
}

// NodeTimelineSegment mirrors internal/nodeinsight.Segment.
type NodeTimelineSegment struct {
	State   string    `json:"state"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Seconds int64     `json:"seconds"`
}

// NodeTimeline mirrors internal/nodeinsight.Timeline.
type NodeTimeline struct {
	From          time.Time             `json:"from"`
	To            time.Time             `json:"to"`
	Segments      []NodeTimelineSegment `json:"segments"`
	Transitions   int                   `json:"transitions"`
	UptimePercent *float64              `json:"uptime_percent,omitempty"`
}

// NodeVersionSkewNode mirrors internal/nodeinsight.NodeSkew.
type NodeVersionSkewNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	OS          string `json:"os,omitempty"`
	Arch        string `json:"arch,omitempty"`
	Online      bool   `json:"online"`
	Class       string `json:"class"`
	NeedsUpdate bool   `json:"needs_update"`
}

// NodeUpgradeStep mirrors internal/nodeinsight.PlanStep.
type NodeUpgradeStep struct {
	Order  int    `json:"order"`
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	From   string `json:"from,omitempty"`
	To     string `json:"to"`
	Note   string `json:"note,omitempty"`
}

// NodeVersionSkew mirrors internal/nodeinsight.SkewReport.
type NodeVersionSkew struct {
	ControlPlaneVersion string                `json:"control_plane_version"`
	Nodes               []NodeVersionSkewNode `json:"nodes"`
	Current             int                   `json:"current"`
	Behind              int                   `json:"behind"`
	Ahead               int                   `json:"ahead"`
	Unknown             int                   `json:"unknown"`
	Plan                []NodeUpgradeStep     `json:"plan"`
}

// GetNodeDoctor calls GET /api/v1/nodes/{id}/doctor.
func (c *Client) GetNodeDoctor(ctx context.Context, id string) (NodeDoctor, error) {
	var out NodeDoctor
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/doctor", nil, &out)
	return out, err
}

// GetNodeTimeline calls GET /api/v1/nodes/{id}/timeline. An empty window uses the server default.
func (c *Client) GetNodeTimeline(ctx context.Context, id, window string) (NodeTimeline, error) {
	path := nodePath(id) + "/timeline"
	if window != "" {
		path += "?window=" + url.QueryEscape(window)
	}
	var out NodeTimeline
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetNodeVersionSkew calls GET /api/v1/nodes/version-skew.
func (c *Client) GetNodeVersionSkew(ctx context.Context) (NodeVersionSkew, error) {
	var out NodeVersionSkew
	err := c.do(ctx, http.MethodGet, nodesCollectionPath()+"/version-skew", nil, &out)
	return out, err
}

// NodeAgentUpdate mirrors internal/nodeinsight.UpdateDecision.
type NodeAgentUpdate struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
	Target  string `json:"target_version,omitempty"`
	Reason  string `json:"reason"`
	// AutoApply is the nodes.agent_auto_update runtime setting.
	AutoApply bool `json:"auto_apply"`
	// Waiting is why a pending update is not applying on its own.
	Waiting string `json:"waiting,omitempty"`
	Held    bool   `json:"held,omitempty"`
}

// GetNodeAgentUpdate calls GET /api/v1/nodes/{id}/agent-update.
func (c *Client) GetNodeAgentUpdate(ctx context.Context, id string) (NodeAgentUpdate, error) {
	var out NodeAgentUpdate
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/agent-update", nil, &out)
	return out, err
}

// NodeAgentUpdateControl calls POST or DELETE on /api/v1/nodes/{id}/agent-update/{hold|override}.
// Actions: hold, unhold, override (a one-time Update anyway request).
func (c *Client) NodeAgentUpdateControl(ctx context.Context, id, action string) error {
	method, leaf := http.MethodPost, action
	if action == "unhold" {
		method, leaf = http.MethodDelete, "hold"
	}
	return c.do(ctx, method, nodePath(id)+"/agent-update/"+leaf, nil, nil)
}
