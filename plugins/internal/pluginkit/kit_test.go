package pluginkit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fast() HTTP {
	return HTTP{Retries: 2, Sleep: func(context.Context, time.Duration) error { return nil }}
}

func post(url string) func() (*http.Request, error) {
	return func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader("{}"))
	}
}

func TestDeliverStatusMapping(t *testing.T) {
	var calls atomic.Int32
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		delivered bool
		retryable bool
		detail    string
		wantCalls int32
	}{
		{"ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }, true, false, "delivered", 1},
		{"401", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) }, false, false, "authentication failed", 1},
		{"429 long wait", func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("Retry-After", "30"); w.WriteHeader(429) }, false, true, "rate limited", 1},
		{"429 short then ok", func(w http.ResponseWriter, _ *http.Request) {
			if calls.Load() == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(429)
				return
			}
			w.WriteHeader(200)
		}, true, false, "delivered", 2},
		{"5xx exhausts retries", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }, false, true, "service error", 3},
		{"400 permanent", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte("bad token-xyz"))
		}, false, false, "rejected", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls.Store(0)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				tc.handler(w, r)
			}))
			defer srv.Close()
			r := fast().Deliver(context.Background(), post(srv.URL), nil, "token-xyz")
			if r.Delivered != tc.delivered || r.Retryable != tc.retryable || !strings.Contains(r.Detail, tc.detail) || calls.Load() != tc.wantCalls {
				t.Fatalf("got %+v calls=%d", r, calls.Load())
			}
			if strings.Contains(r.Detail, "token-xyz") {
				t.Fatalf("secret leaked: %s", r.Detail)
			}
		})
	}
}

func TestDeliverAcceptRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"ok":false}`)) }))
	defer srv.Close()
	r := fast().Deliver(context.Background(), post(srv.URL), func([]byte) (bool, string) { return false, "nope" })
	if r.Delivered || r.Retryable || r.Detail != "nope" {
		t.Fatalf("got %+v", r)
	}
}

func TestDeliverRedactsNetworkError(t *testing.T) {
	r := fast().Deliver(context.Background(), post("http://127.0.0.1:1/hook-SECRET"), nil, "hook-SECRET")
	if r.Delivered || !r.Retryable || strings.Contains(r.Detail, "SECRET") {
		t.Fatalf("got %+v", r)
	}
}

func TestBaseURL(t *testing.T) {
	for raw, ok := range map[string]bool{"https://a.example.com/": true, "http://a.example.com": false, "https://u:p@a.example.com": false, "": false} {
		if _, err := BaseURL(raw, false); (err == nil) != ok {
			t.Errorf("%q: err=%v", raw, err)
		}
	}
	if _, err := BaseURL("http://127.0.0.1:1", true); err != nil {
		t.Errorf("allowHTTP: %v", err)
	}
}

func TestRetryAfterBody(t *testing.T) {
	if d := RetryAfter("", []byte(`{"retry_after_ms":1500}`)); d != 1500*time.Millisecond {
		t.Fatalf("got %s", d)
	}
}
