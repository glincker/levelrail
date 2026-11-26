package apiclient

import (
	"context"
	"net/url"
	"strconv"
)

// DatabaseAuditEntry is one row of GET /databases/{name}/audit.
type DatabaseAuditEntry struct {
	ID            string `json:"id"`
	At            string `json:"at"`
	Database      string `json:"database"`
	Engine        string `json:"engine"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	PrincipalName string `json:"principal_name"`
	Ability       string `json:"ability"`
	Action        string `json:"action"`
	OpKind        string `json:"op_kind"`
	Fingerprint   string `json:"fingerprint"`
	StatementText string `json:"statement_text,omitempty"`
	Rows          int64  `json:"rows"`
	Bytes         int64  `json:"bytes"`
	DurationMs    int64  `json:"duration_ms"`
	Outcome       string `json:"outcome"`
	ErrorClass    string `json:"error_class,omitempty"`
	ClientIP      string `json:"client_ip,omitempty"`
	ClientKind    string `json:"client_kind,omitempty"`
	Branch        string `json:"branch,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
}

// DatabaseAuditPage is one page of the audit with the cursor for the next.
type DatabaseAuditPage struct {
	Items      []DatabaseAuditEntry `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

// DatabaseAuditQuery holds the optional filters of the audit list.
type DatabaseAuditQuery struct {
	Principal string
	Action    string
	Outcome   string
	Since     string
	Until     string
	Cursor    string
	Limit     int
}

// ListDatabaseAudit calls GET /api/v1/databases/{name}/audit.
func (c *Client) ListDatabaseAudit(ctx context.Context, name string, q DatabaseAuditQuery) (DatabaseAuditPage, error) {
	v := url.Values{}
	for k, s := range map[string]string{"principal": q.Principal, "action": q.Action, "outcome": q.Outcome, "since": q.Since, "until": q.Until, "cursor": q.Cursor} {
		if s != "" {
			v.Set(k, s)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/api/v1/databases/" + PathEscape(name) + "/audit"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	return getJSON[DatabaseAuditPage](ctx, c, path)
}
