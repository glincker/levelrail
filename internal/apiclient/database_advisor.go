package apiclient

import "context"

// DatabaseAdvisorFinding is one performance observation; Suggestion is text and is never run by the server.
type DatabaseAdvisorFinding struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Severity   string         `json:"severity"`
	Title      string         `json:"title"`
	Object     string         `json:"object,omitempty"`
	Evidence   map[string]any `json:"evidence,omitempty"`
	Suggestion string         `json:"suggestion,omitempty"`
	Docs       string         `json:"docs,omitempty"`
}

// DatabaseAdvisor is GET /api/v1/databases/{name}/advisor.
type DatabaseAdvisor struct {
	Engine    string                   `json:"engine"`
	CheckedAt string                   `json:"checked_at"`
	Findings  []DatabaseAdvisorFinding `json:"findings"`
	Truncated bool                     `json:"truncated"`
	Cached    bool                     `json:"cached,omitempty"`
}

// GetDatabaseAdvisor calls GET /api/v1/databases/{name}/advisor.
func (c *Client) GetDatabaseAdvisor(ctx context.Context, name string) (DatabaseAdvisor, error) {
	return getJSON[DatabaseAdvisor](ctx, c, "/api/v1/databases/"+PathEscape(name)+"/advisor")
}
