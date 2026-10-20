package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

const hubBase = "/api/v1/migration/hub/sessions"

// HubSource locates a source server. Password travels in the request body
// only and is never stored by the control plane.
type HubSource struct {
	Engine   string `json:"engine"`
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	NodeID   string `json:"node_id,omitempty"`
	// Container names a local database container to read over its own network.
	Container string `json:"container,omitempty"`
}

// HubLocalSource is a database container on the node, with how to reach it.
type HubLocalSource struct {
	Container string `json:"container"`
	Image     string `json:"image"`
	Engine    string `json:"engine"`
	Running   bool   `json:"running"`
	Network   string `json:"network,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	Problem   string `json:"problem,omitempty"`
}

// ListMigrationLocalSources lists database containers on a node.
func (c *Client) ListMigrationLocalSources(ctx context.Context, nodeID string) ([]HubLocalSource, error) {
	var out []HubLocalSource
	path := "/api/v1/migration/hub/local-sources"
	if nodeID != "" {
		path += "?node_id=" + url.QueryEscape(nodeID)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// HubCheck is one preflight finding.
type HubCheck struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	NextAction string `json:"next_action,omitempty"`
}

// HubPreflight is the checks for one database.
type HubPreflight struct {
	Checks          []HubCheck `json:"checks"`
	TargetVersion   string     `json:"target_version"`
	RequiredBytes   int64      `json:"required_bytes"`
	EstimateSeconds int        `json:"estimate_seconds"`
	Blocked         bool       `json:"blocked"`
}

// HubItem is one source database in a session.
type HubItem struct {
	SourceDB      string          `json:"source_db"`
	SizeBytes     int64           `json:"size_bytes"`
	Tables        int             `json:"tables"`
	Extensions    []string        `json:"extensions,omitempty"`
	TargetName    string          `json:"target_name"`
	TargetVersion string          `json:"target_version"`
	Selected      bool            `json:"selected"`
	Status        string          `json:"status"`
	Reason        string          `json:"reason,omitempty"`
	Preflight     HubPreflight    `json:"preflight"`
	Checked       int             `json:"checked"`
	Mismatched    int             `json:"mismatched"`
	TableCounts   []DataCopyTable `json:"table_counts,omitempty"`
}

// HubSummary is the plan totals.
type HubSummary struct {
	Databases       int   `json:"databases"`
	Selected        int   `json:"selected"`
	Blocked         int   `json:"blocked"`
	Warnings        int   `json:"warnings"`
	Verified        int   `json:"verified"`
	Failed          int   `json:"failed"`
	Copying         int   `json:"copying"`
	TotalBytes      int64 `json:"total_bytes"`
	RequiredBytes   int64 `json:"required_bytes"`
	EstimateSeconds int   `json:"estimate_seconds"`
	CanApply        bool  `json:"can_apply"`
}

// HubSession is a migration session with its plan and progress.
type HubSession struct {
	ID            string     `json:"id"`
	Engine        string     `json:"engine"`
	Host          string     `json:"host"`
	Port          int        `json:"port"`
	User          string     `json:"user,omitempty"`
	ServerVersion string     `json:"server_version"`
	FreeBytes     int64      `json:"free_bytes"`
	Step          string     `json:"step"`
	Container     string     `json:"source_container,omitempty"`
	HelperNetwork string     `json:"helper_network,omitempty"`
	PasswordHeld  bool       `json:"password_held"`
	Running       bool       `json:"running"`
	Items         []HubItem  `json:"items"`
	Summary       HubSummary `json:"summary"`
}

// HubSelection changes one item's plan. Nil fields are left alone.
type HubSelection struct {
	SourceDB      string  `json:"source_db"`
	Selected      *bool   `json:"selected,omitempty"`
	TargetName    *string `json:"target_name,omitempty"`
	TargetVersion *string `json:"target_version,omitempty"`
}

// CreateMigrationSession inventories a source server read-only.
func (c *Client) CreateMigrationSession(ctx context.Context, src HubSource) (HubSession, error) {
	var out HubSession
	err := c.do(ctx, http.MethodPost, hubBase, src, &out)
	return out, err
}

// ListMigrationSessions lists sessions without their items.
func (c *Client) ListMigrationSessions(ctx context.Context) ([]HubSession, error) {
	var out []HubSession
	err := c.do(ctx, http.MethodGet, hubBase, nil, &out)
	return out, err
}

// GetMigrationSession returns one session's plan and progress.
func (c *Client) GetMigrationSession(ctx context.Context, id string) (HubSession, error) {
	var out HubSession
	err := c.do(ctx, http.MethodGet, hubBase+"/"+url.PathEscape(id), nil, &out)
	return out, err
}

// SelectMigrationItems updates the selection and names of a session.
func (c *Client) SelectMigrationItems(ctx context.Context, id string, items []HubSelection) (HubSession, error) {
	var out HubSession
	body := struct {
		Items []HubSelection `json:"items"`
	}{Items: items}
	err := c.do(ctx, http.MethodPut, hubBase+"/"+url.PathEscape(id)+"/selection", body, &out)
	return out, err
}

// ApplyMigrationSession starts the bulk copy. A non-empty password replaces
// the one held in memory.
func (c *Client) ApplyMigrationSession(ctx context.Context, id, password string) (HubSession, error) {
	var out HubSession
	body := struct {
		Password string `json:"password,omitempty"`
	}{Password: password}
	err := c.do(ctx, http.MethodPost, hubBase+"/"+url.PathEscape(id)+"/apply", body, &out)
	return out, err
}
