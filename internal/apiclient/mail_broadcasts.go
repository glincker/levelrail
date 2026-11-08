package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// MailSuppression is an address that opted out of announcements.
type MailSuppression struct {
	Email     string `json:"email"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// MailBroadcasts calls GET /api/v1/mail/broadcasts.
func (c *Client) MailBroadcasts(ctx context.Context) ([]MailBroadcast, error) {
	var out struct {
		Broadcasts []MailBroadcast `json:"broadcasts"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/broadcasts", nil, &out)
	return out.Broadcasts, err
}

// MailBroadcastCancel calls POST /api/v1/mail/broadcasts/{id}/cancel.
func (c *Client) MailBroadcastCancel(ctx context.Context, id string) (MailBroadcast, error) {
	var out MailBroadcast
	return out, c.do(ctx, http.MethodPost, "/api/v1/mail/broadcasts/"+url.PathEscape(id)+"/cancel", nil, &out)
}

// MailSuppressions calls GET /api/v1/mail/suppressions.
func (c *Client) MailSuppressions(ctx context.Context) ([]MailSuppression, error) {
	var out struct {
		Suppressions []MailSuppression `json:"suppressions"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/suppressions", nil, &out)
	return out.Suppressions, err
}

// MailSuppressionAdd calls POST /api/v1/mail/suppressions.
func (c *Client) MailSuppressionAdd(ctx context.Context, email, reason string) (MailSuppression, error) {
	var out MailSuppression
	body := map[string]string{"email": email, "reason": reason}
	return out, c.do(ctx, http.MethodPost, "/api/v1/mail/suppressions", body, &out)
}

// MailSuppressionRemove calls DELETE /api/v1/mail/suppressions/{email}.
func (c *Client) MailSuppressionRemove(ctx context.Context, email string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/mail/suppressions/"+url.PathEscape(email), nil, nil)
}
