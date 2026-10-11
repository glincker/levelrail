package apiclient

import (
	"context"
	"net/http"
	"time"
)

// MobilePairingCode is the one-time code a phone exchanges for a device token.
type MobilePairingCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	ServerURL string    `json:"server_url"`
	PairURL   string    `json:"pair_url"`
	QRPayload string    `json:"qr_payload"`
}

// MobileDevice is one paired phone.
type MobileDevice struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	TokenID    string     `json:"token_id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Expired    bool       `json:"expired"`
}

type createMobilePairingRequest struct {
	StepUp *StepUpProof `json:"step_up,omitempty"`
}

// CreateMobilePairing calls POST /api/v1/mobile/pairing-codes with the token owner's step-up proof.
func (c *Client) CreateMobilePairing(ctx context.Context, proof *StepUpProof) (MobilePairingCode, error) {
	var out MobilePairingCode
	err := c.do(ctx, http.MethodPost, "/api/v1/mobile/pairing-codes", createMobilePairingRequest{StepUp: proof}, &out)
	return out, err
}

// ListMobileDevices calls GET /api/v1/mobile/devices.
func (c *Client) ListMobileDevices(ctx context.Context) ([]MobileDevice, error) {
	var out []MobileDevice
	err := c.do(ctx, http.MethodGet, "/api/v1/mobile/devices", nil, &out)
	return out, err
}

// RevokeMobileDevice calls DELETE /api/v1/mobile/devices/{id}.
func (c *Client) RevokeMobileDevice(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/mobile/devices/"+PathEscape(id), nil, nil)
}
