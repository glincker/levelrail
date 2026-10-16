package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// NodeTrustStatus mirrors internal/api's nodeTrustStatus.
type NodeTrustStatus struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// NodeTrustCA mirrors internal/api's nodeTrustCA.
type NodeTrustCA struct {
	Fingerprint   string     `json:"fingerprint,omitempty"`
	PinState      string     `json:"pin_state,omitempty"`
	PinReportedAt *time.Time `json:"pin_reported_at,omitempty"`
}

// NodeTrustCert mirrors internal/api's nodeTrustCert.
type NodeTrustCert struct {
	Fingerprint   string     `json:"fingerprint,omitempty"`
	Serial        string     `json:"serial,omitempty"`
	IssuedAt      *time.Time `json:"issued_at,omitempty"`
	NotAfter      *time.Time `json:"not_after,omitempty"`
	DaysRemaining *int       `json:"days_remaining,omitempty"`
	LifetimeUsed  *float64   `json:"lifetime_used,omitempty"`
	KeyOrigin     string     `json:"key_origin"`
}

// NodeTrustRenewal mirrors internal/api's nodeTrustRenewal.
type NodeTrustRenewal struct {
	NextRenewAt         *time.Time `json:"next_renew_at,omitempty"`
	NextRenewEstimated  bool       `json:"next_renew_estimated,omitempty"`
	LastRenewedAt       *time.Time `json:"last_renewed_at,omitempty"`
	RenewalCount        int        `json:"renewal_count"`
	LastError           string     `json:"last_error,omitempty"`
	LastFailedAt        *time.Time `json:"last_failed_at,omitempty"`
	NextRetryAt         *time.Time `json:"next_retry_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

// NodeTrustValidation mirrors internal/api's nodeTrustValidation.
type NodeTrustValidation struct {
	WindowHours    int        `json:"window_hours"`
	Accepted       int        `json:"accepted"`
	Rejected       int        `json:"rejected"`
	LastAcceptedAt *time.Time `json:"last_accepted_at,omitempty"`
	LastRejectedAt *time.Time `json:"last_rejected_at,omitempty"`
}

// NodeTrust mirrors internal/api's nodeTrustResource.
type NodeTrust struct {
	NodeID     string               `json:"node_id"`
	IsLocal    bool                 `json:"is_local"`
	Status     NodeTrustStatus      `json:"status"`
	CA         *NodeTrustCA         `json:"ca,omitempty"`
	Cert       *NodeTrustCert       `json:"certificate,omitempty"`
	Renewal    *NodeTrustRenewal    `json:"renewal,omitempty"`
	Validation *NodeTrustValidation `json:"validation,omitempty"`
}

// NodeSecurityEvent mirrors internal/api's nodeSecurityEventResource.
type NodeSecurityEvent struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	Outcome     string    `json:"outcome"`
	Reason      string    `json:"reason,omitempty"`
	SourceIP    string    `json:"source_ip,omitempty"`
	Count       int       `json:"count"`
	FirstAt     time.Time `json:"first_at"`
	LastAt      time.Time `json:"last_at"`
	TokenID     string    `json:"token_id,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	Message     string    `json:"message,omitempty"`
	Serial      string    `json:"serial,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	NotBefore   int64     `json:"not_before,omitempty"`
	NotAfter    int64     `json:"not_after,omitempty"`
	Actor       string    `json:"actor,omitempty"`
}

// GetNodeTrust calls GET /api/v1/nodes/{id}/security.
func (c *Client) GetNodeTrust(ctx context.Context, id string) (NodeTrust, error) {
	var out NodeTrust
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/security", nil, &out)
	return out, err
}

// ListNodeSecurityEvents calls GET /api/v1/nodes/{id}/security/events.
// outcome and limit are optional.
func (c *Client) ListNodeSecurityEvents(ctx context.Context, id, outcome string, limit int) ([]NodeSecurityEvent, error) {
	q := url.Values{}
	if outcome != "" {
		q.Set("outcome", outcome)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := nodePath(id) + "/security/events"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []NodeSecurityEvent
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// RenewNodeCert calls POST /api/v1/nodes/{id}/cert/renew.
func (c *Client) RenewNodeCert(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, nodePath(id)+"/cert/renew", nil, nil)
}
