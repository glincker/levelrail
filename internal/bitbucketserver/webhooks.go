package bitbucketserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// webhookName marks the hooks this control plane owns so DeleteRepoWebhook never touches a hook someone else added.
const webhookName = "levelrail"

type webhookRequest struct {
	Name          string            `json:"name"`
	URL           string            `json:"url"`
	Active        bool              `json:"active"`
	Events        []string          `json:"events"`
	Configuration map[string]string `json:"configuration"`
}

// CreateRepoWebhook registers a repo:refs_changed webhook on "PROJECT/slug".
// The secret makes Bitbucket sign each delivery with X-Hub-Signature (sha256=).
func (c *Client) CreateRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL, secret string) error {
	base, err := repoPath(instanceURL, repo)
	if err != nil {
		return err
	}
	body, err := json.Marshal(webhookRequest{ //nolint:gosec // secret configures delivery signing on the provider, it is not leaked
		Name: webhookName, URL: hookURL, Active: true,
		Events:        []string{"repo:refs_changed"},
		Configuration: map[string]string{"secret": secret},
	})
	if err != nil {
		return fmt.Errorf("%s: marshal webhook request: %w", errPrefix, err)
	}
	return c.do(ctx, http.MethodPost, base+"/webhooks", token, bytes.NewReader(body), nil)
}

// DeleteRepoWebhook removes this control plane's webhook pointing at hookURL; it is a no-op when none exists.
func (c *Client) DeleteRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL string) error {
	base, err := repoPath(instanceURL, repo)
	if err != nil {
		return err
	}
	var resp page[struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	}]
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/webhooks?limit=%d", base, pageLimit), token, nil, &resp); err != nil {
		return err
	}
	for _, h := range resp.Values {
		if h.Name == webhookName && h.URL == hookURL {
			return c.do(ctx, http.MethodDelete, base+"/webhooks/"+strconv.Itoa(h.ID), token, nil, nil)
		}
	}
	return nil
}
