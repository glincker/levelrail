package azuredevops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	pushEventType = "git.push"
	hookPublisher = "tfs"
)

type subscription struct {
	ID             string            `json:"id,omitempty"`
	PublisherID    string            `json:"publisherId"`
	EventType      string            `json:"eventType"`
	ResourceVer    string            `json:"resourceVersion"`
	ConsumerID     string            `json:"consumerId"`
	ConsumerAction string            `json:"consumerActionId"`
	PublisherIn    map[string]string `json:"publisherInputs"`
	ConsumerIn     map[string]string `json:"consumerInputs"`
}

// CreateRepoWebhook subscribes a git.push service hook for one repository.
// Service hooks cannot HMAC-sign, so the secret rides as the basic-auth password.
func (c *Client) CreateRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL, secret string) error {
	r, err := c.GetRepo(ctx, instanceURL, token, repo)
	if err != nil {
		return err
	}
	body, err := json.Marshal(subscription{ //nolint:gosec // secret authenticates deliveries on the provider side, it is not leaked
		PublisherID: hookPublisher, EventType: pushEventType, ResourceVer: "1.0",
		ConsumerID: "webHooks", ConsumerAction: "httpRequest",
		PublisherIn: map[string]string{"projectId": r.ProjectID, "repository": r.ID},
		ConsumerIn:  map[string]string{"url": hookURL, "basicAuthUsername": WebhookUser, "basicAuthPassword": secret},
	})
	if err != nil {
		return fmt.Errorf("%s: marshal webhook request: %w", errPrefix, err)
	}
	return c.do(ctx, http.MethodPost, apiURL(instanceURL, "/_apis/hooks/subscriptions", nil), token, bytes.NewReader(body), nil)
}

// DeleteRepoWebhook removes the push subscription pointing at hookURL; it is a no-op when none exists.
func (c *Client) DeleteRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL string) error {
	r, err := c.GetRepo(ctx, instanceURL, token, repo)
	if err != nil {
		return err
	}
	var resp struct {
		Value []subscription `json:"value"`
	}
	q := url.Values{"publisherId": {hookPublisher}, "eventType": {pushEventType}}
	if err := c.do(ctx, http.MethodGet, apiURL(instanceURL, "/_apis/hooks/subscriptions", q), token, nil, &resp); err != nil {
		return err
	}
	for _, s := range resp.Value {
		if s.ConsumerIn["url"] == hookURL && s.PublisherIn["repository"] == r.ID {
			return c.do(ctx, http.MethodDelete, apiURL(instanceURL, "/_apis/hooks/subscriptions/"+url.PathEscape(s.ID), nil), token, nil, nil)
		}
	}
	return nil
}
