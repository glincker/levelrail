package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// JoinCommands are ready-to-paste commands that enroll a server.
type JoinCommands struct {
	Docker string `json:"docker"`
	Binary string `json:"binary"`
}

// CreateNodeJoinTokenRequest optionally tailors the returned join commands.
type CreateNodeJoinTokenRequest struct {
	ControlPlaneAddr string `json:"control_plane_addr,omitempty"`
	NodeName         string `json:"node_name,omitempty"`
	Mesh             bool   `json:"mesh,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Region           string `json:"region,omitempty"`
	Zone             string `json:"zone,omitempty"`
}

// CreateNodeJoinTokenFor calls POST /api/v1/nodes/join-tokens with req.
func (c *Client) CreateNodeJoinTokenFor(ctx context.Context, req CreateNodeJoinTokenRequest) (CreateNodeJoinTokenResponse, error) {
	var out CreateNodeJoinTokenResponse
	err := c.do(ctx, http.MethodPost, nodesCollectionPath()+"/join-tokens", req, &out)
	return out, err
}

// NodeMoveStep is one recorded phase of a node move.
type NodeMoveStep struct {
	Phase   string `json:"phase"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
	At      string `json:"at"`
}

// NodeMoveResource mirrors internal/api's nodeMoveResource.
type NodeMoveResource struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Resource   string         `json:"resource"`
	FromNodeID string         `json:"from_node_id"`
	ToNodeID   string         `json:"to_node_id"`
	Stateful   bool           `json:"stateful"`
	Volumes    []string       `json:"volumes"`
	Phase      string         `json:"phase"`
	Status     string         `json:"status"`
	Error      string         `json:"error,omitempty"`
	DrainID    string         `json:"drain_id,omitempty"`
	Steps      []NodeMoveStep `json:"steps"`
	StartedAt  string         `json:"started_at"`
	UpdatedAt  string         `json:"updated_at"`
	FinishedAt string         `json:"finished_at,omitempty"`
}

// statusRunning is the in-flight status of a node move or drain.
const statusRunning = "running"

// Done reports whether the move reached a final status.
func (m NodeMoveResource) Done() bool { return m.Status != statusRunning }

// DrainBlocked is a resource a drain could not move.
type DrainBlocked struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// NodeDrainResource mirrors internal/api's nodeDrainResource.
type NodeDrainResource struct {
	ID           string             `json:"id"`
	NodeID       string             `json:"node_id"`
	TargetNodeID string             `json:"target_node_id,omitempty"`
	Status       string             `json:"status"`
	Error        string             `json:"error,omitempty"`
	Blocked      []DrainBlocked     `json:"blocked"`
	Moves        []NodeMoveResource `json:"moves"`
	StartedAt    string             `json:"started_at"`
	UpdatedAt    string             `json:"updated_at"`
	FinishedAt   string             `json:"finished_at,omitempty"`
}

// Done reports whether the drain reached a final status.
func (d NodeDrainResource) Done() bool { return d.Status != statusRunning }

// StartMove calls POST /api/v1/{apps|databases}/{name}/move. kind is "app" or
// "database"; an empty nodeID lets the scheduler pick.
func (c *Client) StartMove(ctx context.Context, kind, name, nodeID string) (NodeMoveResource, error) {
	var out NodeMoveResource
	err := c.do(ctx, http.MethodPost, movePath(kind, name)+"/move", map[string]string{"node_id": nodeID}, &out)
	return out, err
}

func movePath(kind, name string) string {
	if kind == "database" {
		return "/api/v1/databases/" + PathEscape(name)
	}
	return "/api/v1/apps/" + PathEscape(name)
}

// GetNodeMove calls GET /api/v1/node-moves/{id}.
func (c *Client) GetNodeMove(ctx context.Context, id string) (NodeMoveResource, error) {
	var out NodeMoveResource
	err := c.do(ctx, http.MethodGet, "/api/v1/node-moves/"+PathEscape(id), nil, &out)
	return out, err
}

// ListNodeMoves calls GET /api/v1/node-moves with optional filters.
func (c *Client) ListNodeMoves(ctx context.Context, kind, resource, status string) ([]NodeMoveResource, error) {
	q := url.Values{}
	for k, v := range map[string]string{"kind": kind, "resource": resource, "status": status} {
		if v != "" {
			q.Set(k, v)
		}
	}
	path := "/api/v1/node-moves"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []NodeMoveResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// StartNodeDrain calls POST /api/v1/nodes/{id}/drain and returns the drain job.
func (c *Client) StartNodeDrain(ctx context.Context, id, targetNodeID string) (NodeDrainResource, error) {
	path := nodePath(id) + "/drain"
	if targetNodeID != "" {
		path += "?" + url.Values{"target_node_id": {targetNodeID}}.Encode()
	}
	var out NodeDrainResource
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}

// NodeDrainPreview is the dry run of a drain: what would move, nothing changed.
type NodeDrainPreview struct {
	DryRun    bool     `json:"dry_run"`
	NodeID    string   `json:"node_id"`
	Services  []string `json:"services"`
	Databases []string `json:"databases"`
}

// PreviewNodeDrain calls POST /api/v1/nodes/{id}/drain?dry_run=true.
func (c *Client) PreviewNodeDrain(ctx context.Context, id string) (NodeDrainPreview, error) {
	var out NodeDrainPreview
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/drain?dry_run=true", nil, &out)
	return out, err
}

// GetNodeDrain calls GET /api/v1/node-drains/{id}.
func (c *Client) GetNodeDrain(ctx context.Context, id string) (NodeDrainResource, error) {
	var out NodeDrainResource
	err := c.do(ctx, http.MethodGet, "/api/v1/node-drains/"+PathEscape(id), nil, &out)
	return out, err
}

// ListNodeDrains calls GET /api/v1/nodes/{id}/drains.
func (c *Client) ListNodeDrains(ctx context.Context, nodeID string) ([]NodeDrainResource, error) {
	var out []NodeDrainResource
	err := c.do(ctx, http.MethodGet, nodePath(nodeID)+"/drains", nil, &out)
	return out, err
}

// NodeLabels is the body of the node labels routes.
type NodeLabels struct {
	Labels map[string]string `json:"labels"`
}

// GetNodeLabels calls GET /api/v1/nodes/{id}/labels.
func (c *Client) GetNodeLabels(ctx context.Context, id string) (NodeLabels, error) {
	var out NodeLabels
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/labels", nil, &out)
	return out, err
}

// SetNodeLabels calls PUT /api/v1/nodes/{id}/labels, replacing the set.
func (c *Client) SetNodeLabels(ctx context.Context, id string, labels map[string]string) (NodeLabels, error) {
	var out NodeLabels
	err := c.do(ctx, http.MethodPut, nodePath(id)+"/labels", NodeLabels{Labels: labels}, &out)
	return out, err
}

// PlacementRejection says why a node is not eligible.
type PlacementRejection struct {
	NodeID string `json:"node_id"`
	Reason string `json:"reason"`
}

// PlacementResource mirrors internal/api's placementResource.
type PlacementResource struct {
	Kind         string               `json:"kind"`
	Name         string               `json:"name"`
	NodeID       string               `json:"node_id"`
	NodeSelector map[string]string    `json:"node_selector"`
	Eligible     []string             `json:"eligible_node_ids"`
	Rejected     []PlacementRejection `json:"rejected"`
}

// GetPlacement calls GET /api/v1/{apps|databases}/{name}/placement.
func (c *Client) GetPlacement(ctx context.Context, kind, name string) (PlacementResource, error) {
	var out PlacementResource
	err := c.do(ctx, http.MethodGet, movePath(kind, name)+"/placement", nil, &out)
	return out, err
}

// SetPlacement calls PUT /api/v1/{apps|databases}/{name}/placement.
func (c *Client) SetPlacement(ctx context.Context, kind, name string, selector map[string]string) (PlacementResource, error) {
	var out PlacementResource
	err := c.do(ctx, http.MethodPut, movePath(kind, name)+"/placement", map[string]map[string]string{"node_selector": selector}, &out)
	return out, err
}

// MeshLink is one direction of a node pair.
type MeshLink struct {
	State         string     `json:"state"`
	LastHandshake *time.Time `json:"last_handshake,omitempty"`
	Endpoint      string     `json:"endpoint,omitempty"`
}

// MeshPair is one node pair's connectivity.
type MeshPair struct {
	A      string   `json:"a"`
	B      string   `json:"b"`
	Status string   `json:"status"`
	AToB   MeshLink `json:"a_to_b"`
	BToA   MeshLink `json:"b_to_a"`
	Hint   string   `json:"hint,omitempty"`
}

// MeshConnectivityNode is one mesh member and whether its view was readable.
type MeshConnectivityNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MeshAddress string `json:"mesh_address"`
	ViewError   string `json:"view_error,omitempty"`
}

// MeshConnectivity mirrors GET /api/v1/mesh/connectivity.
type MeshConnectivity struct {
	Nodes     []MeshConnectivityNode `json:"nodes"`
	Pairs     []MeshPair             `json:"pairs"`
	CheckedAt time.Time              `json:"checked_at"`
}

// GetMeshConnectivity calls GET /api/v1/mesh/connectivity.
func (c *Client) GetMeshConnectivity(ctx context.Context) (MeshConnectivity, error) {
	var out MeshConnectivity
	err := c.do(ctx, http.MethodGet, "/api/v1/mesh/connectivity", nil, &out)
	return out, err
}
