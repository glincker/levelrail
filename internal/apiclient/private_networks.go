package apiclient

import (
	"context"
	"net/http"
)

// PrivateNetworkResource mirrors one entry of GET /api/v1/private-networks.
type PrivateNetworkResource struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	ProjectID   string   `json:"project_id,omitempty"`
	DefaultDeny bool     `json:"default_deny"`
	Members     []string `json:"members"`
	CreatedAt   string   `json:"created_at"`
}

// PrivateNetworkRuleResource is one allow rule between two apps.
type PrivateNetworkRuleResource struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

// PrivateNetworkList is the GET /api/v1/private-networks body.
type PrivateNetworkList struct {
	Networks []PrivateNetworkResource     `json:"networks"`
	Rules    []PrivateNetworkRuleResource `json:"rules"`
}

// PrivateNetworkRequest is the create and update body.
type PrivateNetworkRequest struct {
	Name        string   `json:"name,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	DefaultDeny bool     `json:"default_deny"`
	Members     []string `json:"members"`
}

// PrivateNetworkRuleRequest is the allow rule body.
type PrivateNetworkRuleRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note,omitempty"`
}

// PrivateNetworkMapApp is a graph node.
type PrivateNetworkMapApp struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ProjectID string   `json:"project_id,omitempty"`
	NodeID    string   `json:"node_id,omitempty"`
	Networks  []string `json:"networks"`
}

// PrivateNetworkMapEdge is a graph edge with its verdict and fix.
type PrivateNetworkMapEdge struct {
	A         string `json:"a"`
	B         string `json:"b"`
	Allowed   bool   `json:"allowed"`
	Via       string `json:"via,omitempty"`
	ViaName   string `json:"via_name,omitempty"`
	RuleID    string `json:"rule_id,omitempty"`
	CrossNode bool   `json:"cross_node"`
	Observed  string `json:"observed"`
	Fix       string `json:"fix,omitempty"`
}

// PrivateNetworkMap is the GET /api/v1/private-networks/map body.
type PrivateNetworkMap struct {
	Apps  []PrivateNetworkMapApp  `json:"apps"`
	Edges []PrivateNetworkMapEdge `json:"edges"`
}

// PrivateNetworkChange is the small ack body of a mutation.
type PrivateNetworkChange struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Deleted  bool   `json:"deleted,omitempty"`
	Enforced bool   `json:"enforced"`
}

const privateNetworksPath = "/api/v1/private-networks"

// ListPrivateNetworks calls GET /api/v1/private-networks.
func (c *Client) ListPrivateNetworks(ctx context.Context) (PrivateNetworkList, error) {
	var out PrivateNetworkList
	err := c.do(ctx, http.MethodGet, privateNetworksPath, nil, &out)
	return out, err
}

// CreatePrivateNetwork calls POST /api/v1/private-networks.
func (c *Client) CreatePrivateNetwork(ctx context.Context, req PrivateNetworkRequest) (PrivateNetworkChange, error) {
	var out PrivateNetworkChange
	err := c.do(ctx, http.MethodPost, privateNetworksPath, req, &out)
	return out, err
}

// UpdatePrivateNetwork calls PUT /api/v1/private-networks/{id}.
func (c *Client) UpdatePrivateNetwork(ctx context.Context, id string, req PrivateNetworkRequest) (PrivateNetworkChange, error) {
	var out PrivateNetworkChange
	err := c.do(ctx, http.MethodPut, privateNetworksPath+"/"+PathEscape(id), req, &out)
	return out, err
}

// DeletePrivateNetwork calls DELETE /api/v1/private-networks/{id}.
func (c *Client) DeletePrivateNetwork(ctx context.Context, id string) (PrivateNetworkChange, error) {
	var out PrivateNetworkChange
	err := c.do(ctx, http.MethodDelete, privateNetworksPath+"/"+PathEscape(id), nil, &out)
	return out, err
}

// CreatePrivateNetworkRule calls POST /api/v1/private-networks/rules.
func (c *Client) CreatePrivateNetworkRule(ctx context.Context, req PrivateNetworkRuleRequest) (PrivateNetworkChange, error) {
	var out PrivateNetworkChange
	err := c.do(ctx, http.MethodPost, privateNetworksPath+"/rules", req, &out)
	return out, err
}

// DeletePrivateNetworkRule calls DELETE /api/v1/private-networks/rules/{id}.
func (c *Client) DeletePrivateNetworkRule(ctx context.Context, id string) (PrivateNetworkChange, error) {
	var out PrivateNetworkChange
	err := c.do(ctx, http.MethodDelete, privateNetworksPath+"/rules/"+PathEscape(id), nil, &out)
	return out, err
}

// GetPrivateNetworkMap calls GET /api/v1/private-networks/map.
func (c *Client) GetPrivateNetworkMap(ctx context.Context) (PrivateNetworkMap, error) {
	var out PrivateNetworkMap
	err := c.do(ctx, http.MethodGet, privateNetworksPath+"/map", nil, &out)
	return out, err
}

// ApplyPrivateNetworks calls POST /api/v1/private-networks/apply.
func (c *Client) ApplyPrivateNetworks(ctx context.Context) (PrivateNetworkChange, error) {
	var out PrivateNetworkChange
	err := c.do(ctx, http.MethodPost, privateNetworksPath+"/apply", nil, &out)
	return out, err
}
