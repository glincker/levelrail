package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// StackRequest is the body of the stack plan, apply and rollback calls.
type StackRequest struct {
	Files            []IaCFile         `json:"files,omitempty"`
	Vars             map[string]string `json:"vars,omitempty"`
	Secrets          map[string]string `json:"secrets,omitempty"`
	Project          string            `json:"project,omitempty"`
	Prune            bool              `json:"prune,omitempty"`
	NoDeploy         bool              `json:"no_deploy,omitempty"`
	ContinueOnError  bool              `json:"continue_on_error,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty"`
	GitRepo          string            `json:"git_repo,omitempty"`
	GitRef           string            `json:"git_ref,omitempty"`
	GitSHA           string            `json:"git_sha,omitempty"`
	CIRunURL         string            `json:"ci_run_url,omitempty"`
	IdempotencyKey   string            `json:"idempotency_key,omitempty"`
}

// StackLock is the apply lease held on a stack.
type StackLock struct {
	Holder    string `json:"holder"`
	Revision  int    `json:"revision"`
	ExpiresAt string `json:"expires_at"`
}

// StackResource is one ledger entry of a stack.
type StackResource struct {
	Kind          string `json:"kind"`
	Key           string `json:"key"`
	LiveID        string `json:"live_id"`
	SinceRevision int    `json:"since_revision"`
}

// StackInfo is a stack as the API returns it.
type StackInfo struct {
	Name            string          `json:"name"`
	ProjectID       string          `json:"project_id"`
	Project         string          `json:"project"`
	EnvironmentID   string          `json:"environment_id,omitempty"`
	AllowPrune      bool            `json:"allow_prune"`
	DriftPolicy     string          `json:"drift_policy"`
	PinnedRevision  int             `json:"pinned_revision,omitempty"`
	RequireApproval bool            `json:"require_approval"`
	CreatedAt       string          `json:"created_at"`
	CreatedBy       string          `json:"created_by"`
	LatestRevision  int             `json:"latest_revision"`
	LatestStatus    string          `json:"latest_status,omitempty"`
	Lock            *StackLock      `json:"lock,omitempty"`
	Resources       []StackResource `json:"resources,omitempty"`
	Drift           StackDrift      `json:"drift"`
}

// StackDrift is the latest drift check of a stack; field paths only.
type StackDrift struct {
	Status    string           `json:"status"`
	CheckedAt string           `json:"checked_at,omitempty"`
	Summary   []StackDriftItem `json:"summary,omitempty"`
}

// StackDriftItem names one drifted resource.
type StackDriftItem struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// StackRevision is the summary of one revision.
type StackRevision struct {
	Number         int    `json:"number"`
	Status         string `json:"status"`
	PlanHash       string `json:"plan_hash"`
	RollbackOf     int    `json:"rollback_of,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Actor          string `json:"actor"`
	TokenID        string `json:"token_id,omitempty"`
	GitRepo        string `json:"git_repo,omitempty"`
	GitRef         string `json:"git_ref,omitempty"`
	GitSHA         string `json:"git_sha,omitempty"`
	CIRunURL       string `json:"ci_run_url,omitempty"`
	CreatedAt      string `json:"created_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
	// Rejected marks a failed revision that a reviewer turned down.
	Rejected bool `json:"rejected,omitempty"`
}

// StackRevisionDetail adds the stored documents, plan and result.
type StackRevisionDetail struct {
	StackRevision
	Files  []IaCFile        `json:"files"`
	Plan   *iac.Plan        `json:"plan,omitempty"`
	Result *iac.ApplyResult `json:"result,omitempty"`
}

// StackHistory is the revision list, newest first.
type StackHistory struct {
	Stack          string          `json:"stack"`
	PinnedRevision int             `json:"pinned_revision,omitempty"`
	Revisions      []StackRevision `json:"revisions"`
}

// StackPlanResponse is the result of a stack plan.
type StackPlanResponse struct {
	Stack          string      `json:"stack"`
	StackExists    bool        `json:"stack_exists"`
	PinnedRevision int         `json:"pinned_revision,omitempty"`
	Plan           iac.Plan    `json:"plan"`
	Issues         []iac.Issue `json:"issues,omitempty"`
	Warnings       []string    `json:"warnings,omitempty"`
}

// StackApplyResponse is the result of a stack apply or rollback. The call
// answers 200 even when the revision ended partial or failed.
type StackApplyResponse struct {
	Stack    string           `json:"stack"`
	Revision *StackRevision   `json:"revision,omitempty"`
	Result   *iac.ApplyResult `json:"result,omitempty"`
	Issues   []iac.Issue      `json:"issues,omitempty"`
	Replayed bool             `json:"replayed,omitempty"`
	Noop     bool             `json:"noop,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
	// PendingApproval is true on a 202: the revision is stored and nothing ran.
	PendingApproval bool      `json:"pending_approval,omitempty"`
	ExpiresAt       string    `json:"expires_at,omitempty"`
	Plan            *iac.Plan `json:"plan,omitempty"`
}

// StackApproveRequest is the body of an approval. Secrets are never stored with
// a pending revision, so the approver supplies them here.
type StackApproveRequest struct {
	Secrets          map[string]string `json:"secrets,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty"`
}

// StackRejectResponse is the result of rejecting a pending revision.
type StackRejectResponse struct {
	Stack    string        `json:"stack"`
	Revision StackRevision `json:"revision"`
	Reason   string        `json:"reason,omitempty"`
}

// StackFieldChange is one field of a revision diff.
type StackFieldChange struct {
	Path string `json:"path"`
	Op   string `json:"op"`
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
}

// StackDiffChange is one resource of a revision diff.
type StackDiffChange struct {
	Kind   string             `json:"kind"`
	Name   string             `json:"name"`
	Scope  string             `json:"scope,omitempty"`
	Action string             `json:"action"`
	Fields []StackFieldChange `json:"fields,omitempty"`
}

// StackDiff compares a revision with another one or with live state.
type StackDiff struct {
	Stack    string            `json:"stack"`
	Revision int               `json:"revision"`
	Against  string            `json:"against"`
	PlanHash string            `json:"plan_hash,omitempty"`
	Changes  []StackDiffChange `json:"changes"`
}

// StackDeleteResult reports what a stack delete did.
type StackDeleteResult struct {
	Stack     string   `json:"stack"`
	Mode      string   `json:"mode"`
	Destroyed []string `json:"destroyed,omitempty"`
	Forgotten []string `json:"forgotten,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	DataKept  []string `json:"data_kept,omitempty"`
}

// StackCreateRequest is the body of POST /api/v1/stacks.
type StackCreateRequest struct {
	Name            string `json:"name"`
	Project         string `json:"project"`
	AllowPrune      *bool  `json:"allow_prune,omitempty"`
	DriftPolicy     string `json:"drift_policy,omitempty"`
	RequireApproval *bool  `json:"require_approval,omitempty"`
}

// StackSettings is the body of PATCH /api/v1/stacks/{name}; nil leaves a field.
type StackSettings struct {
	AllowPrune      *bool   `json:"allow_prune,omitempty"`
	DriftPolicy     *string `json:"drift_policy,omitempty"`
	RequireApproval *bool   `json:"require_approval,omitempty"`
}

func stackPath(name string, rest ...string) string {
	p := "/api/v1/stacks/" + url.PathEscape(name)
	for _, r := range rest {
		p += "/" + r
	}
	return p
}

// stackCall runs one call and returns the server JSON untouched so --json can
// pass it through, plus the typed decode.
func stackCall[T any](ctx context.Context, c *Client, method, path string, body any) (T, json.RawMessage, error) {
	var zero, out T
	var raw json.RawMessage
	if err := c.do(ctx, method, path, body, &raw); err != nil {
		return zero, nil, asIssues(err)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return zero, raw, fmt.Errorf("decode stack response: %w", err)
		}
	}
	return out, raw, nil
}

// ListStacks calls GET /api/v1/stacks.
func (c *Client) ListStacks(ctx context.Context) ([]StackInfo, json.RawMessage, error) {
	out, raw, err := stackCall[struct {
		Stacks []StackInfo `json:"stacks"`
	}](ctx, c, http.MethodGet, "/api/v1/stacks", nil)
	return out.Stacks, raw, err
}

// GetStack calls GET /api/v1/stacks/{name}.
func (c *Client) GetStack(ctx context.Context, name string) (StackInfo, json.RawMessage, error) {
	return stackCall[StackInfo](ctx, c, http.MethodGet, stackPath(name), nil)
}

// CreateStack calls POST /api/v1/stacks.
func (c *Client) CreateStack(ctx context.Context, req StackCreateRequest) (StackInfo, json.RawMessage, error) {
	return stackCall[StackInfo](ctx, c, http.MethodPost, "/api/v1/stacks", req)
}

// UpdateStackSettings calls PATCH /api/v1/stacks/{name}.
func (c *Client) UpdateStackSettings(ctx context.Context, name string, s StackSettings) (StackInfo, json.RawMessage, error) {
	return stackCall[StackInfo](ctx, c, http.MethodPatch, stackPath(name), s)
}

// DeleteStack calls DELETE /api/v1/stacks/{name}?mode=.
func (c *Client) DeleteStack(ctx context.Context, name, mode string) (StackDeleteResult, json.RawMessage, error) {
	return c.DeleteStackData(ctx, name, mode, false)
}

// DeleteStackData is DeleteStack that can also delete the databases the stack owns.
func (c *Client) DeleteStackData(ctx context.Context, name, mode string, deleteData bool) (StackDeleteResult, json.RawMessage, error) {
	q := url.Values{}
	if mode != "" {
		q.Set("mode", mode)
	}
	if deleteData {
		q.Set("delete_data", "true")
	}
	p := stackPath(name)
	if len(q) > 0 {
		p += "?" + q.Encode()
	}
	return stackCall[StackDeleteResult](ctx, c, http.MethodDelete, p, nil)
}

// PlanStack calls POST /api/v1/stacks/{name}/plan.
func (c *Client) PlanStack(ctx context.Context, name string, req StackRequest) (StackPlanResponse, json.RawMessage, error) {
	return stackCall[StackPlanResponse](ctx, c, http.MethodPost, stackPath(name, "plan"), req)
}

// ApplyStack calls POST /api/v1/stacks/{name}/apply.
func (c *Client) ApplyStack(ctx context.Context, name string, req StackRequest) (StackApplyResponse, json.RawMessage, error) {
	return stackCall[StackApplyResponse](ctx, c, http.MethodPost, stackPath(name, "apply"), req)
}

// StackHistory calls GET /api/v1/stacks/{name}/revisions.
func (c *Client) StackHistory(ctx context.Context, name string, limit int) (StackHistory, json.RawMessage, error) {
	p := stackPath(name, "revisions")
	if limit > 0 {
		p += fmt.Sprintf("?limit=%d", limit)
	}
	return stackCall[StackHistory](ctx, c, http.MethodGet, p, nil)
}

// StackRevision calls GET /api/v1/stacks/{name}/revisions/{n}.
func (c *Client) StackRevision(ctx context.Context, name string, n int) (StackRevisionDetail, json.RawMessage, error) {
	return stackCall[StackRevisionDetail](ctx, c, http.MethodGet, stackPath(name, "revisions", fmt.Sprint(n)), nil)
}

// StackRevisionDiff calls GET /api/v1/stacks/{name}/revisions/{n}/diff.
func (c *Client) StackRevisionDiff(ctx context.Context, name string, n int, against string) (StackDiff, json.RawMessage, error) {
	p := stackPath(name, "revisions", fmt.Sprint(n), "diff")
	if against != "" {
		p += "?against=" + url.QueryEscape(against)
	}
	return stackCall[StackDiff](ctx, c, http.MethodGet, p, nil)
}

// RollbackStack calls POST /api/v1/stacks/{name}/revisions/{n}/rollback.
func (c *Client) RollbackStack(ctx context.Context, name string, n int, req StackRequest) (StackApplyResponse, json.RawMessage, error) {
	return stackCall[StackApplyResponse](ctx, c, http.MethodPost, stackPath(name, "revisions", fmt.Sprint(n), "rollback"), req)
}

// ApproveStack calls POST /api/v1/stacks/{name}/revisions/{n}/approve.
func (c *Client) ApproveStack(ctx context.Context, name string, n int, req StackApproveRequest) (StackApplyResponse, json.RawMessage, error) {
	return stackCall[StackApplyResponse](ctx, c, http.MethodPost, stackPath(name, "revisions", fmt.Sprint(n), "approve"), req)
}

// RejectStack calls POST /api/v1/stacks/{name}/revisions/{n}/reject.
func (c *Client) RejectStack(ctx context.Context, name string, n int, reason string) (StackRejectResponse, json.RawMessage, error) {
	return stackCall[StackRejectResponse](ctx, c, http.MethodPost, stackPath(name, "revisions", fmt.Sprint(n), "reject"), map[string]string{"reason": reason})
}

// PinStack calls PUT /api/v1/stacks/{name}/pin.
func (c *Client) PinStack(ctx context.Context, name string, n int) (StackInfo, json.RawMessage, error) {
	return stackCall[StackInfo](ctx, c, http.MethodPut, stackPath(name, "pin"), map[string]int{"revision": n})
}

// UnpinStack calls DELETE /api/v1/stacks/{name}/pin.
func (c *Client) UnpinStack(ctx context.Context, name string) (StackInfo, json.RawMessage, error) {
	return stackCall[StackInfo](ctx, c, http.MethodDelete, stackPath(name, "pin"), nil)
}

// UnlockStack calls POST /api/v1/stacks/{name}/unlock.
func (c *Client) UnlockStack(ctx context.Context, name string) (StackInfo, json.RawMessage, error) {
	return stackCall[StackInfo](ctx, c, http.MethodPost, stackPath(name, "unlock"), nil)
}
