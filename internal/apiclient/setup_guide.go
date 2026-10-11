package apiclient

import (
	"context"
	"net/http"
)

// SetupGuideProgress counts done items of a multi-item setup step.
type SetupGuideProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// SetupGuideStep is one entry of the Get started checklist.
type SetupGuideStep struct {
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	State           string              `json:"state"`
	Optional        bool                `json:"optional"`
	EstimateMinutes int                 `json:"estimate_minutes"`
	Href            string              `json:"href"`
	BlockedBy       []string            `json:"blocked_by,omitempty"`
	Reason          string              `json:"reason,omitempty"`
	Progress        *SetupGuideProgress `json:"progress,omitempty"`
	Manual          bool                `json:"manual,omitempty"`
}

// SetupGuideResource is GET /api/v1/setup/guide.
type SetupGuideResource struct {
	Steps         []SetupGuideStep `json:"steps"`
	ReadyPercent  int              `json:"ready_percent"`
	Done          int              `json:"done"`
	Total         int              `json:"total"`
	NextStep      string           `json:"next_step,omitempty"`
	Complete      bool             `json:"complete"`
	Hidden        bool             `json:"hidden"`
	ServerAddress string           `json:"server_address,omitempty"`
}

// GetSetupGuide calls GET /api/v1/setup/guide.
func (c *Client) GetSetupGuide(ctx context.Context) (SetupGuideResource, error) {
	var out SetupGuideResource
	err := c.do(ctx, http.MethodGet, "/api/v1/setup/guide", nil, &out)
	return out, err
}

// SetSetupGuideStep calls PUT /api/v1/setup/guide/steps/{id}; state is
// "skipped", "done" or "todo" (clears an override).
func (c *Client) SetSetupGuideStep(ctx context.Context, id, state string) (SetupGuideResource, error) {
	var out SetupGuideResource
	body := struct {
		State string `json:"state"`
	}{State: state}
	err := c.do(ctx, http.MethodPut, "/api/v1/setup/guide/steps/"+PathEscape(id), body, &out)
	return out, err
}

// SetSetupGuideHidden calls PUT /api/v1/setup/guide/visibility.
func (c *Client) SetSetupGuideHidden(ctx context.Context, hidden bool) (SetupGuideResource, error) {
	var out SetupGuideResource
	body := struct {
		Hidden bool `json:"hidden"`
	}{Hidden: hidden}
	err := c.do(ctx, http.MethodPut, "/api/v1/setup/guide/visibility", body, &out)
	return out, err
}

// ResetSetupGuide calls POST /api/v1/setup/guide/reset.
func (c *Client) ResetSetupGuide(ctx context.Context) (SetupGuideResource, error) {
	var out SetupGuideResource
	err := c.do(ctx, http.MethodPost, "/api/v1/setup/guide/reset", nil, &out)
	return out, err
}
