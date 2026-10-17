package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// NodeCertResource mirrors internal/api's nodeCertResource.
type NodeCertResource struct {
	// State is ok, expiring, critical, expired, revoked, or unknown.
	State              string     `json:"state"`
	NotAfter           *time.Time `json:"not_after,omitempty"`
	DaysRemaining      *int       `json:"days_remaining,omitempty"`
	RenewedAt          *time.Time `json:"renewed_at,omitempty"`
	Generation         int        `json:"generation"`
	KeyOrigin          string     `json:"key_origin"`
	PreviousValidUntil *time.Time `json:"previous_valid_until,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	WarningDays        int        `json:"warning_days"`
	CriticalDays       int        `json:"critical_days"`
	// renewal is unexported so the MCP output schemas, which are derived from
	// this type and have almost no token headroom, do not grow.
	renewal *NodeCertRenewal
}

// UnmarshalJSON reads the wire form, including the renewal block.
func (c *NodeCertResource) UnmarshalJSON(b []byte) error {
	type plain NodeCertResource
	aux := struct {
		*plain
		Renewal *NodeCertRenewal `json:"renewal"`
	}{plain: (*plain)(c)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err //nolint:wrapcheck // thin decoder wrapper, caller adds context
	}
	c.renewal = aux.Renewal
	return nil
}

// Renewal returns the agent's last reported renewal failure, or nil.
func (c *NodeCertResource) Renewal() *NodeCertRenewal { return c.renewal }

// SetRenewal sets the renewal block, for building values in tests.
func (c *NodeCertResource) SetRenewal(r *NodeCertRenewal) { c.renewal = r }

// NodeCertRenewal mirrors internal/api's nodeCertRenewal.
type NodeCertRenewal struct {
	LastError           string     `json:"last_error"`
	FailedAt            *time.Time `json:"failed_at,omitempty"`
	NextRetryAt         *time.Time `json:"next_retry_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

// NodeAgentResource mirrors internal/api's nodeAgentResource.
type NodeAgentResource struct {
	Version             string     `json:"version,omitempty"`
	Commit              string     `json:"commit,omitempty"`
	OS                  string     `json:"os,omitempty"`
	Arch                string     `json:"arch,omitempty"`
	ReportedAt          *time.Time `json:"reported_at,omitempty"`
	Outdated            bool       `json:"outdated"`
	MinVersion          string     `json:"min_version,omitempty"`
	ControlPlaneVersion string     `json:"control_plane_version"`
}

// NodeReenrollTokenResponse mirrors internal/api's reenrollTokenResponse:
// the only time the token is shown.
type NodeReenrollTokenResponse struct {
	Token         string    `json:"token"`
	NodeID        string    `json:"node_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	CAFingerprint string    `json:"ca_fingerprint,omitempty"`
	AgentBinary   string    `json:"agent_binary"`
}

// CreateNodeReenrollToken calls POST /api/v1/nodes/{id}/reenroll-token.
func (c *Client) CreateNodeReenrollToken(ctx context.Context, id string) (NodeReenrollTokenResponse, error) {
	var out NodeReenrollTokenResponse
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/reenroll-token", nil, &out)
	return out, err
}

// RevokeNodeCert calls POST /api/v1/nodes/{id}/revoke-cert.
func (c *Client) RevokeNodeCert(ctx context.Context, id string) (NodeResource, error) {
	var out NodeResource
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/revoke-cert", nil, &out)
	return out, err
}
