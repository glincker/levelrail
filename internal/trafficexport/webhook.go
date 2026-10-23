package trafficexport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// WebhookConfig configures a generic newline-delimited JSON sink.
type WebhookConfig struct {
	URL     string
	Headers map[string]string
	HTTP    HTTPOptions
}

// WebhookSink posts events as application/x-ndjson.
type WebhookSink struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// NewWebhookSink validates the target and returns the sink.
func NewWebhookSink(cfg WebhookConfig) (*WebhookSink, error) {
	o := cfg.HTTP.withDefaults()
	if err := checkTarget(cfg.URL, o); err != nil {
		return nil, fmt.Errorf("webhook sink: %w", err)
	}
	return &WebhookSink{url: cfg.URL, headers: cfg.Headers, client: newClient(o)}, nil
}

// Name identifies the sink in stats and logs.
func (s *WebhookSink) Name() string { return "webhook" }

// Send posts one ndjson body.
func (s *WebhookSink) Send(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range events {
		if err := enc.Encode(&events[i]); err != nil {
			return fmt.Errorf("webhook sink: encode event: %w", err)
		}
	}
	return postBody(ctx, s.client, s.Name(), s.url, "application/x-ndjson", s.headers, buf.Bytes())
}
