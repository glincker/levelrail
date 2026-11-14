package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// StackPlanJob is an async cloud plan job: the 202 answer of a plan and each poll.
type StackPlanJob struct {
	ID      string             `json:"id"`
	Stack   string             `json:"stack"`
	Status  string             `json:"status"`
	Code    string             `json:"code,omitempty"`
	Error   string             `json:"error,omitempty"`
	PollURL string             `json:"poll_url,omitempty"`
	Result  *StackPlanResponse `json:"result,omitempty"`
}

// PlanStackAsync is PlanStack that tells a 202 job answer from a finished plan. job is nil for a sync plan.
func (c *Client) PlanStackAsync(ctx context.Context, name string, req StackRequest, supersede bool) (plan StackPlanResponse, job *StackPlanJob, raw json.RawMessage, err error) {
	p := stackPath(name, "plan")
	if supersede {
		p += "?" + url.Values{"supersede": {"true"}}.Encode()
	}
	_, raw, err = stackCall[json.RawMessage](ctx, c, http.MethodPost, p, req)
	if err != nil {
		return plan, nil, nil, err
	}
	var probe StackPlanJob
	if json.Unmarshal(raw, &probe) == nil && probe.ID != "" && probe.Status != "" {
		return plan, &probe, raw, nil
	}
	plan, _, err = decodeStackRaw[StackPlanResponse](raw)
	return plan, nil, raw, err
}

// StackPlanJobStatus calls GET /api/v1/stacks/{name}/plan-jobs/{id}.
func (c *Client) StackPlanJobStatus(ctx context.Context, name, id string) (StackPlanJob, json.RawMessage, error) {
	return stackCall[StackPlanJob](ctx, c, http.MethodGet, stackPath(name, "plan-jobs", id), nil)
}

func decodeStackRaw[T any](raw json.RawMessage) (T, json.RawMessage, error) {
	var out T
	err := json.Unmarshal(raw, &out)
	return out, raw, err
}

// StackQuarantineRow is one unresolved quarantined cloud state row, never the state itself.
type StackQuarantineRow struct {
	ID        int64  `json:"id"`
	Account   string `json:"account"`
	Reason    string `json:"reason"`
	Fence     int64  `json:"fence"`
	CreatedAt string `json:"created_at"`
}

// StackQuarantineList is the answer of the quarantine listing.
type StackQuarantineList struct {
	Stack      string               `json:"stack"`
	Quarantine []StackQuarantineRow `json:"quarantine"`
}

// ListStackCloudQuarantine calls GET /api/v1/stacks/{name}/cloud-state/quarantine.
func (c *Client) ListStackCloudQuarantine(ctx context.Context, name string) (StackQuarantineList, json.RawMessage, error) {
	return stackCall[StackQuarantineList](ctx, c, http.MethodGet, stackPath(name, "cloud-state", "quarantine"), nil)
}

// ResolveStackCloudQuarantine calls POST /api/v1/stacks/{name}/cloud-state/quarantine/{id}/resolve.
func (c *Client) ResolveStackCloudQuarantine(ctx context.Context, name, id, confirm string) (json.RawMessage, error) {
	_, raw, err := stackCall[json.RawMessage](ctx, c, http.MethodPost, stackPath(name, "cloud-state", "quarantine", id, "resolve"), map[string]string{"confirm": confirm})
	return raw, err
}
