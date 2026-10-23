package trafficexport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

var allowLocal = HTTPOptions{AllowPrivate: true, Timeout: 2 * time.Second}

func testEvent(i int) Event {
	return Event{
		Time: time.Date(2026, 10, 22, 12, 0, i, 0, time.UTC), App: "shop", Host: "shop.example.com",
		Method: "GET", Path: "/cart", Status: 200, DurationMS: 12.5, BytesIn: 10, BytesOut: 2048,
		RequestID: fmt.Sprintf("req%d", i), TraceID: "0af7651916cd43dd8448eb211c80319c",
	}
}

type capture struct {
	mu     sync.Mutex
	paths  []string
	heads  []http.Header
	bodies [][]byte
}

func (c *capture) handler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.paths = append(c.paths, r.URL.Path)
		c.heads = append(c.heads, r.Header.Clone())
		c.bodies = append(c.bodies, b)
		c.mu.Unlock()
		w.WriteHeader(status)
	}
}

func TestPostHogSink(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(c.handler(200))
	defer srv.Close()
	s, err := NewPostHogSink(PostHogConfig{Host: srv.URL + "/", Token: "phc_secret", HTTP: allowLocal})
	if err != nil {
		t.Fatal(err)
	}
	withVisitor := testEvent(1)
	withVisitor.VisitorID = "vis123"
	if err := s.Send(context.Background(), []Event{testEvent(0), withVisitor}); err != nil {
		t.Fatal(err)
	}
	if len(c.bodies) != 1 || c.paths[0] != "/batch" {
		t.Fatalf("paths %v", c.paths)
	}
	var got struct {
		APIKey string `json:"api_key"`
		Batch  []struct {
			Event      string         `json:"event"`
			DistinctID string         `json:"distinct_id"`
			Timestamp  string         `json:"timestamp"`
			Properties map[string]any `json:"properties"`
		} `json:"batch"`
	}
	if err := json.Unmarshal(c.bodies[0], &got); err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "phc_secret" || len(got.Batch) != 2 {
		t.Fatalf("body %s", c.bodies[0])
	}
	b0, b1 := got.Batch[0], got.Batch[1]
	if b0.Event != "http_request" || b0.DistinctID != "req0" || b1.DistinctID != "vis123" {
		t.Fatalf("distinct ids %q %q", b0.DistinctID, b1.DistinctID)
	}
	if _, err := time.Parse(time.RFC3339, b0.Timestamp); err != nil {
		t.Fatalf("timestamp %q: %v", b0.Timestamp, err)
	}
	p := b0.Properties
	if p["$current_url"] != "https://shop.example.com/cart" || p["$process_person_profile"] != false || p["status"] != float64(200) {
		t.Fatalf("properties %v", p)
	}
	if _, ok := p["visitor_id"]; ok {
		t.Fatal("visitor_id must be absent when empty")
	}
}

func TestPostHogSinkSplitsAtBodyCap(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(c.handler(200))
	defer srv.Close()
	opts := allowLocal
	opts.MaxBodyBytes = 1500
	s, err := NewPostHogSink(PostHogConfig{Host: srv.URL, Token: "t", HTTP: opts})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]Event, 10)
	for i := range events {
		events[i] = testEvent(i)
	}
	if err := s.Send(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, b := range c.bodies {
		if len(b) > opts.MaxBodyBytes {
			t.Fatalf("body %d bytes exceeds cap", len(b))
		}
		var got struct {
			Batch []json.RawMessage `json:"batch"`
		}
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		total += len(got.Batch)
	}
	if len(c.bodies) < 2 || total != len(events) {
		t.Fatalf("requests %d events %d", len(c.bodies), total)
	}
}

func TestPostHogSinkRequiresToken(t *testing.T) {
	if _, err := NewPostHogSink(PostHogConfig{Host: PostHogHostEU}); err == nil {
		t.Fatal("want error for missing token")
	}
}

func TestOTLPSink(t *testing.T) {
	tests := []struct {
		name     string
		cfg      func(base string) OTLPConfig
		wantPath string
	}{
		{"default path", func(b string) OTLPConfig { return OTLPConfig{Endpoint: b + "/", HTTP: allowLocal} }, "/v1/logs"},
		{"full url override", func(b string) OTLPConfig { return OTLPConfig{URL: b + "/i/v1/logs", HTTP: allowLocal} }, "/i/v1/logs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &capture{}
			srv := httptest.NewServer(c.handler(200))
			defer srv.Close()
			cfg := tc.cfg(srv.URL)
			cfg.Headers = map[string]string{"Authorization": "Bearer abc"}
			s, err := NewOTLPSink(cfg)
			if err != nil {
				t.Fatal(err)
			}
			e := testEvent(0)
			e.Status = 503
			if err := s.Send(context.Background(), []Event{e}); err != nil {
				t.Fatal(err)
			}
			if c.paths[0] != tc.wantPath {
				t.Fatalf("path %q", c.paths[0])
			}
			if c.heads[0].Get("Content-Type") != "application/x-protobuf" || c.heads[0].Get("Authorization") != "Bearer abc" {
				t.Fatalf("headers %v", c.heads[0])
			}
			var req collogs.ExportLogsServiceRequest
			if err := proto.Unmarshal(c.bodies[0], &req); err != nil {
				t.Fatal(err)
			}
			recs := req.ResourceLogs[0].ScopeLogs[0].LogRecords
			if len(recs) != 1 {
				t.Fatalf("records %d", len(recs))
			}
			attrs := map[string]*commonpb.AnyValue{}
			for _, kv := range recs[0].Attributes {
				attrs[kv.Key] = kv.Value
			}
			if attrs["http.request.method"].GetStringValue() != "GET" ||
				attrs["url.path"].GetStringValue() != "/cart" ||
				attrs["http.response.status_code"].GetIntValue() != 503 ||
				attrs["server.address"].GetStringValue() != "shop.example.com" ||
				attrs["http.server.request.duration"].GetDoubleValue() != 0.0125 {
				t.Fatalf("attrs %v", attrs)
			}
			if len(recs[0].TraceId) != 16 || len(recs[0].SpanId) != 0 || recs[0].SeverityText != "ERROR" {
				t.Fatalf("record %v", recs[0])
			}
		})
	}
}

func TestWebhookSink(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(c.handler(204))
	defer srv.Close()
	s, err := NewWebhookSink(WebhookConfig{URL: srv.URL + "/hook", Headers: map[string]string{"X-Key": "k"}, HTTP: allowLocal})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), []Event{testEvent(0), testEvent(1)}); err != nil {
		t.Fatal(err)
	}
	if c.heads[0].Get("Content-Type") != "application/x-ndjson" || c.heads[0].Get("X-Key") != "k" {
		t.Fatalf("headers %v", c.heads[0])
	}
	sc := bufio.NewScanner(bytes.NewReader(c.bodies[0]))
	lines := 0
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil || e.App != "shop" {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		lines++
	}
	if lines != 2 {
		t.Fatalf("lines %d", lines)
	}
}

func TestSinkStatusErrors(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{500, true}, {503, true}, {429, true}, {408, true}, {400, false}, {401, false}, {404, false},
	}
	for _, tc := range tests {
		srv := httptest.NewServer((&capture{}).handler(tc.status))
		s, err := NewWebhookSink(WebhookConfig{URL: srv.URL, HTTP: allowLocal})
		if err != nil {
			t.Fatal(err)
		}
		err = s.Send(context.Background(), []Event{testEvent(0)})
		srv.Close()
		var se *StatusError
		if !errors.As(err, &se) || se.Code != tc.status || se.Retryable() != tc.retryable {
			t.Fatalf("status %d: err %v", tc.status, err)
		}
	}
}

func TestSinkErrorsHideURL(t *testing.T) {
	s, err := NewWebhookSink(WebhookConfig{URL: "http://127.0.0.1:1/hook?token=SECRET", HTTP: allowLocal})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Send(context.Background(), []Event{testEvent(0)})
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error must exist and hide the url: %v", err)
	}
}

func TestSSRFGuard(t *testing.T) {
	srv := httptest.NewServer((&capture{}).handler(200))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	hostnameURL := "http://localhost:" + u.Port()

	tests := []struct {
		name string
		url  string
	}{
		{"loopback ipv4", "http://127.0.0.1:9/x"},
		{"loopback ipv6", "http://[::1]:9/x"},
		{"link local metadata", "http://169.254.169.254/latest"},
		{"unspecified", "http://0.0.0.0:9/x"},
	}
	for _, tc := range tests {
		t.Run("constructor "+tc.name, func(t *testing.T) {
			if _, err := NewWebhookSink(WebhookConfig{URL: tc.url}); !errors.Is(err, errBlockedTarget) {
				t.Fatalf("want blocked, got %v", err)
			}
		})
	}

	t.Run("hostname resolving to loopback is blocked at dial", func(t *testing.T) {
		s, err := NewWebhookSink(WebhookConfig{URL: hostnameURL})
		if err != nil {
			t.Fatal(err)
		}
		err = s.Send(context.Background(), []Event{testEvent(0)})
		if err == nil || !strings.Contains(err.Error(), "loopback or link-local") {
			t.Fatalf("want blocked dial, got %v", err)
		}
	})
	t.Run("allow private permits loopback", func(t *testing.T) {
		s, err := NewWebhookSink(WebhookConfig{URL: hostnameURL, HTTP: allowLocal})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Send(context.Background(), []Event{testEvent(0)}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("bad scheme", func(t *testing.T) {
		if _, err := NewWebhookSink(WebhookConfig{URL: "file:///etc/passwd", HTTP: allowLocal}); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestRedirectsNotFollowed(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer target.Close()
	redir := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	defer redir.Close()
	s, err := NewWebhookSink(WebhookConfig{URL: redir.URL, HTTP: allowLocal})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Send(context.Background(), []Event{testEvent(0)})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusFound || hit {
		t.Fatalf("err %v hit %v", err, hit)
	}
}

func TestHTTPOptionsFromEnv(t *testing.T) {
	t.Setenv(EnvTimeout, "3s")
	t.Setenv(EnvAllowPrivate, "1")
	t.Setenv(EnvMaxBodyBytes, "bogus")
	o := HTTPOptionsFromEnv()
	if o.Timeout != 3*time.Second || !o.AllowPrivate || o.MaxBodyBytes != defaultMaxBodyBytes {
		t.Fatalf("%+v", o)
	}
	t.Setenv(EnvTimeout, "")
	t.Setenv(EnvAllowPrivate, "")
	o = HTTPOptionsFromEnv()
	if o.Timeout != defaultTimeout || o.AllowPrivate {
		t.Fatalf("%+v", o)
	}
}
