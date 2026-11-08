package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppProtection is the protection state of one app.
type AppProtection struct {
	Protected  bool `json:"protected"`
	Protection *struct {
		Reason      string `json:"reason"`
		ProtectedBy string `json:"protected_by"`
		ProtectedAt string `json:"protected_at"`
	} `json:"protection,omitempty"`
}

func protectionPath(name string) string {
	return "/api/v1/apps/" + url.PathEscape(name) + "/protection"
}

// GetAppProtection reads the app's protection state.
func (c *Client) GetAppProtection(ctx context.Context, name string) (AppProtection, error) {
	var out AppProtection
	err := c.do(ctx, http.MethodGet, protectionPath(name), nil, &out)
	return out, err
}

// ProtectApp protects the app with a reason.
func (c *Client) ProtectApp(ctx context.Context, name, reason string) (AppProtection, error) {
	var out AppProtection
	err := c.do(ctx, http.MethodPut, protectionPath(name), map[string]string{"reason": reason}, &out)
	return out, err
}

// UnprotectApp lifts the protection.
func (c *Client) UnprotectApp(ctx context.Context, name string) (AppProtection, error) {
	var out AppProtection
	err := c.do(ctx, http.MethodDelete, protectionPath(name), nil, &out)
	return out, err
}
