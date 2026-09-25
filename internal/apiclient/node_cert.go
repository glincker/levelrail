package apiclient

import (
	"context"
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
