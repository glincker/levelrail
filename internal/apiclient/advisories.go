package apiclient

import (
	"context"
	"net/http"
	"time"
)

// AdvisoryEntry is one advisory in the configured feed.
type AdvisoryEntry struct {
	ID        string `json:"id"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
	FixedIn   string `json:"fixed_in,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// AdvisoryMatch is one advisory that applies to one running image.
type AdvisoryMatch struct {
	App        string `json:"app"`
	Image      string `json:"image"`
	Digest     string `json:"digest,omitempty"`
	AdvisoryID string `json:"advisory_id"`
	Severity   string `json:"severity"`
	Summary    string `json:"summary"`
	FixedIn    string `json:"fixed_in,omitempty"`
	Reference  string `json:"reference,omitempty"`
	Hint       string `json:"hint"`
}

// AdvisoriesResource is GET /api/v1/security/advisories.
type AdvisoriesResource struct {
	Enabled     bool            `json:"enabled"`
	Source      string          `json:"source,omitempty"`
	Signature   string          `json:"signature,omitempty"`
	Error       string          `json:"error,omitempty"`
	GeneratedAt time.Time       `json:"generated_at,omitempty"`
	LoadedAt    time.Time       `json:"loaded_at,omitempty"`
	Entries     []AdvisoryEntry `json:"entries"`
	Matches     []AdvisoryMatch `json:"matches"`
}

// GetAdvisories returns the feed state and the running images that match it.
func (c *Client) GetAdvisories(ctx context.Context) (AdvisoriesResource, error) {
	var out AdvisoriesResource
	err := c.do(ctx, http.MethodGet, "/api/v1/security/advisories", nil, &out)
	return out, err
}
