package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ExternalDatabaseHealth mirrors the API's last probe outcome.
type ExternalDatabaseHealth struct {
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	LatencyMs int    `json:"latency_ms"`
	CheckedAt string `json:"checked_at,omitempty"`
}

// ExternalDatabase mirrors internal/api's externalDatabaseResource. It never
// carries a password.
type ExternalDatabase struct {
	Name            string                  `json:"name"`
	Engine          string                  `json:"engine"`
	Host            string                  `json:"host"`
	Port            int                     `json:"port"`
	Username        string                  `json:"username,omitempty"`
	Database        string                  `json:"database,omitempty"`
	TLSMode         string                  `json:"tls_mode"`
	Network         string                  `json:"network,omitempty"`
	NodeID          string                  `json:"node_id,omitempty"`
	ProjectID       string                  `json:"project_id,omitempty"`
	SourceContainer string                  `json:"source_container,omitempty"`
	HasPassword     bool                    `json:"has_password"`
	Health          *ExternalDatabaseHealth `json:"health,omitempty"`
}

// ExternalDatabaseRequest is the connect, test and adopt body.
type ExternalDatabaseRequest struct {
	Name         string `json:"name,omitempty"`
	Engine       string `json:"engine,omitempty"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	Database     string `json:"database,omitempty"`
	AuthDatabase string `json:"auth_database,omitempty"`
	TLSMode      string `json:"tls_mode,omitempty"`
	Network      string `json:"network,omitempty"`
	NodeID       string `json:"node_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	Container    string `json:"container,omitempty"`
}

// ExternalDatabaseProbe is one connectivity probe result.
type ExternalDatabaseProbe struct {
	Status    string   `json:"status"`
	Reason    string   `json:"reason,omitempty"`
	LatencyMs int      `json:"latency_ms"`
	Version   string   `json:"version,omitempty"`
	User      string   `json:"user,omitempty"`
	Database  string   `json:"database,omitempty"`
	Databases []string `json:"databases,omitempty"`
}

// ExternalDatabaseCandidate is a running database container that can be adopted.
type ExternalDatabaseCandidate struct {
	ContainerID   string   `json:"container_id"`
	Container     string   `json:"container"`
	Image         string   `json:"image"`
	Engine        string   `json:"engine"`
	Port          int      `json:"port"`
	SuggestedHost string   `json:"suggested_host,omitempty"`
	Network       string   `json:"network,omitempty"`
	Networks      []string `json:"networks,omitempty"`
	PublishedPort int      `json:"published_port,omitempty"`
	SuggestedUser string   `json:"suggested_user,omitempty"`
	Note          string   `json:"note,omitempty"`
}

const externalDatabasesPath = "/api/v1/external-databases"

// ConnectExternalDatabase calls POST /api/v1/external-databases.
func (c *Client) ConnectExternalDatabase(ctx context.Context, req ExternalDatabaseRequest) (ExternalDatabase, error) {
	var out ExternalDatabase
	err := c.do(ctx, http.MethodPost, externalDatabasesPath, req, &out)
	return out, err
}

// TestExternalDatabase calls POST /api/v1/external-databases/test.
func (c *Client) TestExternalDatabase(ctx context.Context, req ExternalDatabaseRequest) (ExternalDatabaseProbe, error) {
	var out ExternalDatabaseProbe
	err := c.do(ctx, http.MethodPost, externalDatabasesPath+"/test", req, &out)
	return out, err
}

// AdoptExternalDatabase calls POST /api/v1/external-databases/adopt.
func (c *Client) AdoptExternalDatabase(ctx context.Context, req ExternalDatabaseRequest) (ExternalDatabase, error) {
	var out ExternalDatabase
	err := c.do(ctx, http.MethodPost, externalDatabasesPath+"/adopt", req, &out)
	return out, err
}

// ListExternalDatabaseCandidates calls GET /api/v1/external-databases/candidates.
func (c *Client) ListExternalDatabaseCandidates(ctx context.Context, nodeID string) ([]ExternalDatabaseCandidate, error) {
	var out []ExternalDatabaseCandidate
	err := c.do(ctx, http.MethodGet, externalDatabasesPath+"/candidates?node_id="+url.QueryEscape(nodeID), nil, &out)
	return out, err
}

// ListExternalDatabases calls GET /api/v1/external-databases.
func (c *Client) ListExternalDatabases(ctx context.Context) ([]ExternalDatabase, error) {
	var out []ExternalDatabase
	err := c.do(ctx, http.MethodGet, externalDatabasesPath, nil, &out)
	return out, err
}

// ProbeExternalDatabase calls POST /api/v1/external-databases/{name}/probe.
func (c *Client) ProbeExternalDatabase(ctx context.Context, name string) (ExternalDatabaseProbe, error) {
	var out ExternalDatabaseProbe
	err := c.do(ctx, http.MethodPost, externalDatabasesPath+"/"+PathEscape(name)+"/probe", nil, &out)
	return out, err
}

// DeleteExternalDatabase calls DELETE /api/v1/external-databases/{name}. It
// removes only the local record; the remote database is untouched.
func (c *Client) DeleteExternalDatabase(ctx context.Context, name string, force bool) error {
	path := externalDatabasesPath + "/" + PathEscape(name)
	if force {
		path += "?force=true"
	}
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
