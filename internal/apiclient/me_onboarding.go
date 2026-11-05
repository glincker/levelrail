package apiclient

import (
	"context"
	"net/http"
	"time"
)

// MeOnboarding is the signed-in user's first-run flow state from /api/v1/me/onboarding.
type MeOnboarding struct {
	State         string     `json:"state"`
	CurrentStep   string     `json:"current_step"`
	FlowVersion   int        `json:"flow_version"`
	Enabled       bool       `json:"enabled"`
	StartedAt     *time.Time `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	DismissedAt   *time.Time `json:"dismissed_at"`
	Milestone     string     `json:"milestone"`
	NextMilestone string     `json:"next_milestone"`
}

// GetMeOnboarding calls GET /api/v1/me/onboarding.
func (c *Client) GetMeOnboarding(ctx context.Context) (MeOnboarding, error) {
	var out MeOnboarding
	err := c.do(ctx, http.MethodGet, "/api/v1/me/onboarding", nil, &out)
	return out, err
}

// SetMeOnboarding calls PUT /api/v1/me/onboarding; step may be empty to keep the current one.
func (c *Client) SetMeOnboarding(ctx context.Context, state, step string) (MeOnboarding, error) {
	var out MeOnboarding
	body := struct {
		State       string `json:"state"`
		CurrentStep string `json:"current_step,omitempty"`
	}{State: state, CurrentStep: step}
	err := c.do(ctx, http.MethodPut, "/api/v1/me/onboarding", body, &out)
	return out, err
}

// ResetMeOnboarding calls POST /api/v1/me/onboarding/reset.
func (c *Client) ResetMeOnboarding(ctx context.Context) (MeOnboarding, error) {
	var out MeOnboarding
	err := c.do(ctx, http.MethodPost, "/api/v1/me/onboarding/reset", nil, &out)
	return out, err
}
