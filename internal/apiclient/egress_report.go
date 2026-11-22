package apiclient

import (
	"context"
	"time"
)

// EgressReportName is one name an app asked for and whether its allowlist would refuse it.
type EgressReportName struct {
	Name      string    `json:"name"`
	Count     int       `json:"count"`
	LastSeen  time.Time `json:"last_seen"`
	WouldDeny bool      `json:"would_deny"`
	Reason    string    `json:"reason,omitempty"`
}

// EgressReport is the body of GET /api/v1/apps/{name}/egress-report.
type EgressReport struct {
	App       string             `json:"app"`
	Enabled   bool               `json:"enabled"`
	Privacy   string             `json:"privacy"`
	Mode      string             `json:"mode,omitempty"`
	Allow     []string           `json:"allow,omitempty"`
	Message   string             `json:"message,omitempty"`
	Names     []EgressReportName `json:"names"`
	Truncated int                `json:"truncated,omitempty"`
}

// AppEgressReport calls GET /api/v1/apps/{name}/egress-report.
func (c *Client) AppEgressReport(ctx context.Context, name string) (EgressReport, error) {
	return getJSON[EgressReport](ctx, c, "/api/v1/apps/"+PathEscape(name)+"/egress-report")
}
