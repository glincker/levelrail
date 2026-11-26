package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DatabaseBranchNode mirrors one node of GET /api/v1/databases/{name}/branches.
type DatabaseBranchNode struct {
	Name            string `json:"name"`
	Parent          string `json:"parent"`
	Root            string `json:"root"`
	Mode            string `json:"mode"`
	Source          string `json:"source"`
	State           string `json:"state"`
	StateReason     string `json:"state_reason,omitempty"`
	ClonePath       string `json:"clone_path,omitempty"`
	ClonePathReason string `json:"clone_path_reason,omitempty"`
	Protected       bool   `json:"protected"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	ExpiryPreset    string `json:"expiry_preset,omitempty"`
	SizeBytes       int64  `json:"size_bytes"`
	LastActiveAt    string `json:"last_active_at,omitempty"`
	BranchedAt      string `json:"branched_at,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// DatabaseBranchTree is the branch tree of one root database.
type DatabaseBranchTree struct {
	Root     string               `json:"root"`
	Branches []DatabaseBranchNode `json:"branches"`
}

// CreateLiveDatabaseBranchRequest is the live copy form of POST /api/v1/databases/{name}/branches.
type CreateLiveDatabaseBranchRequest struct {
	Name      string `json:"name"`
	Mode      string `json:"mode,omitempty"`
	Parent    string `json:"parent,omitempty"`
	TTL       string `json:"ttl,omitempty"`
	Protected bool   `json:"protected,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// PatchDatabaseBranchRequest changes expiry or protection; nil fields stay as they are.
type PatchDatabaseBranchRequest struct {
	TTL       *string `json:"ttl,omitempty"`
	Protected *bool   `json:"protected,omitempty"`
}

func branchPath(db, branch string) string {
	return "/api/v1/databases/" + PathEscape(db) + "/branches/" + PathEscape(branch)
}

// CreateLiveDatabaseBranch copies the live parent into a new branch.
func (c *Client) CreateLiveDatabaseBranch(ctx context.Context, db string, req CreateLiveDatabaseBranchRequest) (DatabaseBranchNode, error) {
	var out DatabaseBranchNode
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(db)+"/branches", req, &out)
	return out, err
}

// ListDatabaseBranches returns the whole tree of the database's root.
func (c *Client) ListDatabaseBranches(ctx context.Context, db string) (DatabaseBranchTree, error) {
	return getJSON[DatabaseBranchTree](ctx, c, "/api/v1/databases/"+PathEscape(db)+"/branches")
}

// GetDatabaseBranch returns one branch.
func (c *Client) GetDatabaseBranch(ctx context.Context, db, branch string) (DatabaseBranchNode, error) {
	return getJSON[DatabaseBranchNode](ctx, c, branchPath(db, branch))
}

// PatchDatabaseBranch extends expiry or toggles protection.
func (c *Client) PatchDatabaseBranch(ctx context.Context, db, branch string, req PatchDatabaseBranchRequest) (DatabaseBranchNode, error) {
	var out DatabaseBranchNode
	err := c.do(ctx, http.MethodPatch, branchPath(db, branch), req, &out)
	return out, err
}

// DeleteDatabaseBranch deletes a branch and its data; confirm must be the branch name.
func (c *Client) DeleteDatabaseBranch(ctx context.Context, db, branch, confirm string) error {
	return c.do(ctx, http.MethodDelete, branchPath(db, branch)+"?confirm="+PathEscape(confirm), nil, nil)
}

// ResetDatabaseBranch clones the parent's current data into the branch again.
func (c *Client) ResetDatabaseBranch(ctx context.Context, db, branch string) (DatabaseBranchNode, error) {
	var out DatabaseBranchNode
	err := c.do(ctx, http.MethodPost, branchPath(db, branch)+"/reset", map[string]string{"confirm": branch}, &out)
	return out, err
}

// PromoteDatabaseBranch detaches a ready branch from its parent.
func (c *Client) PromoteDatabaseBranch(ctx context.Context, db, branch string) (DatabaseBranchNode, error) {
	var out DatabaseBranchNode
	err := c.do(ctx, http.MethodPost, branchPath(db, branch)+"/promote", map[string]string{"confirm": branch}, &out)
	return out, err
}

// SchemaDiffChange is one difference in a branch schema diff.
type SchemaDiffChange struct {
	Object string `json:"object"`
	Kind   string `json:"kind"`
	Schema string `json:"schema"`
	Table  string `json:"table,omitempty"`
	Name   string `json:"name"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// DatabaseBranchSchemaDiff mirrors GET /api/v1/databases/{name}/branches/{branch}/schema-diff.
type DatabaseBranchSchemaDiff struct {
	Branch  string `json:"branch"`
	Against string `json:"against"`
	Summary struct {
		Added   int `json:"added"`
		Removed int `json:"removed"`
		Changed int `json:"changed"`
	} `json:"summary"`
	Changes   []SchemaDiffChange `json:"changes"`
	SQL       string             `json:"sql"`
	Truncated bool               `json:"truncated"`
}

// GetDatabaseBranchSchemaDiff compares a branch with its parent, or with against (another database of the tree).
func (c *Client) GetDatabaseBranchSchemaDiff(ctx context.Context, db, branch, against string) (DatabaseBranchSchemaDiff, error) {
	path := branchPath(db, branch) + "/schema-diff"
	if against != "" {
		path += "?against=" + url.QueryEscape(against)
	}
	return getJSON[DatabaseBranchSchemaDiff](ctx, c, path)
}
