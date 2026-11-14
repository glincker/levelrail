package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
)

// StackSyncConfig is the sync settings object of a stack. Apply, prune and
// deploy have no omitempty so an off flag is always sent as an explicit false.
type StackSyncConfig struct {
	RepoURL         string `json:"repo_url"`
	Branch          string `json:"branch"`
	Path            string `json:"path"`
	CredentialRef   string `json:"credential_ref,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	WebhookOnly     bool   `json:"webhook_only"`
	Enabled         bool   `json:"enabled"`
	Apply           bool   `json:"apply"`
	Prune           bool   `json:"prune"`
	Deploy          bool   `json:"deploy"`
}

// StackSyncStatus is the latest sync result of a stack.
type StackSyncStatus struct {
	Commit     string         `json:"commit,omitempty"`
	CheckedAt  string         `json:"checked_at,omitempty"`
	PlanHash   string         `json:"plan_hash,omitempty"`
	Counts     map[string]int `json:"counts,omitempty"`
	Pending    bool           `json:"pending"`
	Code       string         `json:"code,omitempty"`
	Detail     string         `json:"detail,omitempty"`
	AppliedSHA string         `json:"applied_commit,omitempty"`
	Revision   int            `json:"revision,omitempty"`
}

// StackSyncInfo is GET and PUT /api/v1/stacks/{name}/sync.
type StackSyncInfo struct {
	Config           *StackSyncConfig `json:"config"`
	WebhookSecretSet bool             `json:"webhook_secret_set"`
	Status           *StackSyncStatus `json:"status,omitempty"`
}

// StackSyncSecret is the one time answer of a webhook secret rotation.
type StackSyncSecret struct {
	Secret     string `json:"secret"`
	SyncURL    string `json:"sync_url"`
	RefreshURL string `json:"refresh_url"`
}

// GetStackSync calls GET /api/v1/stacks/{name}/sync.
func (c *Client) GetStackSync(ctx context.Context, name string) (StackSyncInfo, json.RawMessage, error) {
	return stackCall[StackSyncInfo](ctx, c, http.MethodGet, stackPath(name, "sync"), nil)
}

// PutStackSync calls PUT /api/v1/stacks/{name}/sync.
func (c *Client) PutStackSync(ctx context.Context, name string, cfg StackSyncConfig) (StackSyncInfo, json.RawMessage, error) {
	return stackCall[StackSyncInfo](ctx, c, http.MethodPut, stackPath(name, "sync"), cfg)
}

// RefreshStackSync calls POST /api/v1/stacks/{name}/sync/refresh.
func (c *Client) RefreshStackSync(ctx context.Context, name string) (StackSyncStatus, json.RawMessage, error) {
	return stackCall[StackSyncStatus](ctx, c, http.MethodPost, stackPath(name, "sync", "refresh"), nil)
}

// RotateStackSyncSecret calls POST /api/v1/stacks/{name}/sync/webhook-secret/rotate.
func (c *Client) RotateStackSyncSecret(ctx context.Context, name string) (StackSyncSecret, json.RawMessage, error) {
	return stackCall[StackSyncSecret](ctx, c, http.MethodPost, stackPath(name, "sync", "webhook-secret", "rotate"), nil)
}
