package apiclient

import (
	"context"
	"net/http"
	"time"
)

// RolloutProgress is a rolling deploy's last recorded wave and replica counts.
type RolloutProgress struct {
	Active         bool      `json:"active"`
	Phase          string    `json:"phase"`
	Wave           int       `json:"wave"`
	Waves          int       `json:"waves"`
	Desired        int       `json:"desired"`
	Updated        int       `json:"updated"`
	Ready          int       `json:"ready"`
	Unavailable    int       `json:"unavailable"`
	Surge          int       `json:"surge"`
	MaxSurge       int       `json:"max_surge"`
	MaxUnavailable int       `json:"max_unavailable"`
	Message        string    `json:"message,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RolloutResource is GET /api/v1/apps/{name}/rollout.
type RolloutResource struct {
	App      string `json:"app"`
	Strategy string `json:"strategy"`
	Replicas int    `json:"replicas"`
	Policy   struct {
		MaxSurge       string `json:"max_surge,omitempty"`
		MaxUnavailable string `json:"max_unavailable,omitempty"`
		MinAvailable   string `json:"min_available,omitempty"`
	} `json:"policy"`
	Resolved struct {
		MaxSurge       int `json:"max_surge"`
		MaxUnavailable int `json:"max_unavailable"`
		MinAvailable   int `json:"min_available"`
	} `json:"resolved"`
	Progress *RolloutProgress `json:"progress"`
}

// GetRollout calls GET /api/v1/apps/{name}/rollout.
func (c *Client) GetRollout(ctx context.Context, appName string) (RolloutResource, error) {
	var out RolloutResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/rollout", nil, &out)
	return out, err
}
