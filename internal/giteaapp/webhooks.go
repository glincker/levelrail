package giteaapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Webhook is one registered repository hook.
type Webhook struct {
	ID     int64
	URL    string
	Active bool
	Events []string
}

type hookResponse struct {
	ID     int64    `json:"id"`
	Active bool     `json:"active"`
	Events []string `json:"events"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
}

func toWebhook(h hookResponse) Webhook {
	return Webhook{ID: h.ID, URL: h.Config.URL, Active: h.Active, Events: h.Events}
}

// ListRepoWebhooks lists the hooks of fullName ("owner/repo").
func (c *Client) ListRepoWebhooks(ctx context.Context, instanceURL, accessToken, fullName string) ([]Webhook, error) {
	var out []Webhook
	for page := 1; page <= listPageCap; page++ {
		var resp []hookResponse
		u := fmt.Sprintf("%s/repos/%s/hooks?limit=%d&page=%d", apiBaseURL(instanceURL), fullName, listPerPage, page)
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, h := range resp {
			out = append(out, toWebhook(h))
		}
		if len(resp) < listPerPage {
			break
		}
	}
	return out, nil
}

// EnsureRepoWebhook registers the hook once: when a hook already points at
// hookURL it is updated in place (new secret, events, active) instead of
// stacking a duplicate that would deliver every push twice.
func (c *Client) EnsureRepoWebhook(ctx context.Context, instanceURL, accessToken, fullName, hookURL, secret string) (Webhook, error) {
	existing, err := c.ListRepoWebhooks(ctx, instanceURL, accessToken, fullName)
	if err != nil {
		return Webhook{}, err
	}
	payload, err := json.Marshal(createWebhookRequest{ //nolint:gosec // secret configures delivery signing, not a leaked credential
		Type:   "gitea",
		Config: giteaWebhookConfig{URL: hookURL, ContentType: "json", Secret: secret},
		Events: []string{"push", "pull_request"},
		Active: true,
	})
	if err != nil {
		return Webhook{}, fmt.Errorf("giteaapp: marshal webhook request: %w", err)
	}
	var out hookResponse
	base := apiBaseURL(instanceURL) + "/repos/" + fullName + "/hooks"
	for _, h := range existing {
		if h.URL != hookURL {
			continue
		}
		u := fmt.Sprintf("%s/%d", base, h.ID)
		if err := c.do(ctx, http.MethodPatch, u, "Bearer "+accessToken, bytes.NewReader(payload), &out); err != nil {
			return Webhook{}, err
		}
		return toWebhook(out), nil
	}
	if err := c.do(ctx, http.MethodPost, base, "Bearer "+accessToken, bytes.NewReader(payload), &out); err != nil {
		return Webhook{}, err
	}
	return toWebhook(out), nil
}

// DeleteRepoWebhook removes one hook by id.
func (c *Client) DeleteRepoWebhook(ctx context.Context, instanceURL, accessToken, fullName string, hookID int64) error {
	u := fmt.Sprintf("%s/repos/%s/hooks/%d", apiBaseURL(instanceURL), fullName, hookID)
	return c.do(ctx, http.MethodDelete, u, "Bearer "+accessToken, nil, nil)
}

// TestRepoWebhook asks the server to deliver a push event to the hook now,
// which proves the hook is reachable and signed without needing a real push.
func (c *Client) TestRepoWebhook(ctx context.Context, instanceURL, accessToken, fullName string, hookID int64) error {
	u := fmt.Sprintf("%s/repos/%s/hooks/%d/tests", apiBaseURL(instanceURL), fullName, hookID)
	return c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, nil, nil)
}

// VerifySignature checks a delivery body against the signature header
// (X-Gitea-Signature or X-Forgejo-Signature: hex HMAC-SHA256, optionally
// "sha256=" prefixed) in constant time.
func VerifySignature(secret string, body []byte, header string) bool {
	header = strings.TrimPrefix(strings.TrimSpace(header), "sha256=")
	got, err := hex.DecodeString(header)
	if err != nil || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
