package apiclient

import (
	"context"
	"net/http"
	"time"
)

// SignInCode mirrors internal/api's signInCodeItem: a waiting code request,
// without the code itself.
type SignInCode struct {
	ID          string    `json:"id"`
	RequesterIP string    `json:"requester_ip"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Revealable  bool      `json:"revealable"`
}

// SignInApproval mirrors internal/api's signInApprovalItem. MatchOptions
// holds the three numbers to choose from; only one is on the waiting browser.
type SignInApproval struct {
	ID           string    `json:"id"`
	RequesterIP  string    `json:"requester_ip"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	MatchOptions []int     `json:"match_options"`
}

// SignInRequests mirrors internal/api's signInRequestsResponse.
type SignInRequests struct {
	Codes     []SignInCode     `json:"codes"`
	Approvals []SignInApproval `json:"approvals"`
}

// RevealedLoginCode mirrors internal/api's revealLoginCodeResponse.
type RevealedLoginCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TrustedDevice mirrors internal/api's trustedDeviceResource.
type TrustedDevice struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

// CodeLoginSettings mirrors internal/api's codeLoginSettingsResource.
type CodeLoginSettings struct {
	Admins            bool `json:"admins"`
	Others            bool `json:"others"`
	Saved             bool `json:"saved"`
	NewDeviceApproval bool `json:"new_device_approval"`
}

// CodeLoginSettingsUpdate is the PUT body; nil fields keep their value.
type CodeLoginSettingsUpdate struct {
	Admins *bool `json:"admins,omitempty"`
	Others *bool `json:"others,omitempty"`
}

// ListSignInRequests calls GET /api/v1/auth/sign-in-requests. A token must
// belong to a user and hold write:sensitive or signin:approve.
func (c *Client) ListSignInRequests(ctx context.Context) (SignInRequests, error) {
	var out SignInRequests
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/sign-in-requests", nil, &out)
	return out, err
}

// RevealLoginCode calls POST /api/v1/auth/sign-in-requests/codes/{id}/reveal.
// A token must hold signin:approve.
func (c *Client) RevealLoginCode(ctx context.Context, id string) (RevealedLoginCode, error) {
	var out RevealedLoginCode
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/sign-in-requests/codes/"+PathEscape(id)+"/reveal", nil, &out)
	return out, err
}

type loginApprovalDecision struct {
	Match int `json:"match"`
}

// DecideLoginApproval calls POST /api/v1/auth/login-approvals/{id}/approve
// with the number the waiting browser shows, or /deny. A token must hold
// signin:approve.
func (c *Client) DecideLoginApproval(ctx context.Context, id string, approve bool, match int) error {
	if !approve {
		return c.do(ctx, http.MethodPost, "/api/v1/auth/login-approvals/"+PathEscape(id)+"/deny", nil, nil)
	}
	return c.do(ctx, http.MethodPost, "/api/v1/auth/login-approvals/"+PathEscape(id)+"/approve", loginApprovalDecision{Match: match}, nil)
}

// ListTrustedDevices calls GET /api/v1/auth/trusted-devices.
func (c *Client) ListTrustedDevices(ctx context.Context) ([]TrustedDevice, error) {
	var out struct {
		Devices []TrustedDevice `json:"devices"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/trusted-devices", nil, &out)
	return out.Devices, err
}

// RevokeTrustedDevice calls DELETE /api/v1/auth/trusted-devices/{id}.
func (c *Client) RevokeTrustedDevice(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/trusted-devices/"+PathEscape(id), nil, nil)
}

// GetCodeLoginSettings calls GET /api/v1/settings/auth/code-login.
func (c *Client) GetCodeLoginSettings(ctx context.Context) (CodeLoginSettings, error) {
	var out CodeLoginSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/auth/code-login", nil, &out)
	return out, err
}

// UpdateCodeLoginSettings calls PUT /api/v1/settings/auth/code-login (root).
func (c *Client) UpdateCodeLoginSettings(ctx context.Context, in CodeLoginSettingsUpdate) (CodeLoginSettings, error) {
	var out CodeLoginSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/auth/code-login", in, &out)
	return out, err
}
