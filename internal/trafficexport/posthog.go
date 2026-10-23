package trafficexport

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// PostHog cloud ingestion hosts.
const (
	PostHogHostUS = "https://us.i.posthog.com"
	PostHogHostEU = "https://eu.i.posthog.com"
)

// PostHogConfig configures the PostHog events sink. Host is PostHogHostUS,
// PostHogHostEU or a self-hosted base URL.
type PostHogConfig struct {
	Host  string
	Token string
	HTTP  HTTPOptions
}

// PostHogSink posts http_request events to PostHog's /batch endpoint.
type PostHogSink struct {
	url     string
	token   string
	maxBody int
	ns      string
	client  *http.Client
}

// NewPostHogSink validates the host and returns the sink.
func NewPostHogSink(cfg PostHogConfig) (*PostHogSink, error) {
	o := cfg.HTTP.withDefaults()
	if cfg.Token == "" {
		return nil, fmt.Errorf("posthog sink: token is required")
	}
	host := strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
	if host == "" {
		host = PostHogHostUS
	}
	if err := checkTarget(host, o); err != nil {
		return nil, fmt.Errorf("posthog sink: %w", err)
	}
	return &PostHogSink{url: host + "/batch", token: cfg.Token, maxBody: o.MaxBodyBytes, ns: o.Namespace, client: newClient(o)}, nil
}

// Name identifies the sink in stats and logs.
func (s *PostHogSink) Name() string { return "posthog" }

type posthogEvent struct {
	Event      string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Timestamp  string         `json:"timestamp"`
	Properties map[string]any `json:"properties"`
}

func toPostHog(ns string, e Event) posthogEvent {
	id := e.VisitorID
	if id == "" {
		id = e.RequestID
	}
	if id == "" {
		id = e.App
	}
	p := map[string]any{
		"app": e.App, "host": e.Host, "method": e.Method, "path": e.Path, "status": e.Status,
		"duration_ms": e.DurationMS, "bytes_in": e.BytesIn, "bytes_out": e.BytesOut,
		"request_id": e.RequestID, "upstream_error": e.UpstreamError, "cold_start": e.ColdStart,
		"$current_url": "https://" + e.Host + e.Path, "$process_person_profile": false,
		"$lib": ns,
	}
	for k, v := range map[string]string{
		"trace_id": e.TraceID, "visitor_id": e.VisitorID, "referrer_host": e.ReferrerHost, "ua_family": e.UAFamily,
		"client_ip": e.ClientIP, "user_agent": e.UserAgent,
	} {
		if v != "" {
			p[k] = v
		}
	}
	return posthogEvent{
		Event: "http_request", DistinctID: id, Timestamp: e.Time.UTC().Format(time.RFC3339Nano), Properties: p,
	}
}

// Send posts the events, splitting into several requests when one would
// exceed the body cap.
func (s *PostHogSink) Send(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	tok, err := json.Marshal(s.token)
	if err != nil {
		return fmt.Errorf("posthog sink: encode token: %w", err)
	}
	prefix := []byte(`{"api_key":` + string(tok) + `,"batch":[`)
	const suffix = `]}`
	overhead := len(prefix) + len(suffix)

	body := append([]byte(nil), prefix...)
	count := 0
	flush := func() error {
		if count == 0 {
			return nil
		}
		out := append(body, suffix...)
		body, count = append(body[:0], prefix...), 0
		return postBody(ctx, s.client, s.Name(), s.url, "application/json", nil, out)
	}
	for _, e := range events {
		item, err := json.Marshal(toPostHog(s.ns, e))
		if err != nil {
			return fmt.Errorf("posthog sink: encode event: %w", err)
		}
		if overhead+len(item) > s.maxBody {
			slog.Warn("trafficexport: event exceeds body cap, skipped", "sink", s.Name(), "request_id", e.RequestID)
			continue
		}
		if count > 0 && len(body)+1+len(item)+len(suffix) > s.maxBody {
			if err := flush(); err != nil {
				return err
			}
		}
		if count > 0 {
			body = append(body, ',')
		}
		body = append(body, item...)
		count++
	}
	return flush()
}
