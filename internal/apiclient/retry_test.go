package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func retryServer(t *testing.T, status int, retryAfter string, failures int32) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= failures {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"slow down"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func retryClient(url string, slept *[]time.Duration, p RetryPolicy) *Client {
	return NewClient(url, "tok", WithRetry(p), WithSleep(func(_ context.Context, d time.Duration) error {
		*slept = append(*slept, d)
		return nil
	}))
}

var testPolicy = RetryPolicy{MaxRetries: 3, MaxWait: time.Minute, BaseDelay: time.Second}

func TestRetry_HonorsRetryAfterSeconds(t *testing.T) {
	srv, calls := retryServer(t, http.StatusTooManyRequests, "2", 2)
	var slept []time.Duration
	var out map[string]any
	if err := retryClient(srv.URL, &slept, testPolicy).do(context.Background(), http.MethodGet, "/x", nil, &out); err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 3 || len(slept) != 2 {
		t.Fatalf("calls = %d sleeps = %v, want 3 and 2", calls.Load(), slept)
	}
	for _, d := range slept {
		if d < 2*time.Second || d > 2500*time.Millisecond {
			t.Errorf("sleep %v outside Retry-After plus <=25%% jitter", d)
		}
	}
}

func TestRetry_HonorsRetryAfterHTTPDate(t *testing.T) {
	when := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	srv, _ := retryServer(t, http.StatusServiceUnavailable, when, 1)
	var slept []time.Duration
	if err := retryClient(srv.URL, &slept, testPolicy).do(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(slept) != 1 || slept[0] < 25*time.Second || slept[0] > 40*time.Second {
		t.Errorf("sleeps = %v, want about 30s", slept)
	}
}

func TestRetry_PlainPostIsNeverRetried(t *testing.T) {
	srv, calls := retryServer(t, http.StatusTooManyRequests, "1", 5)
	var slept []time.Duration
	err := retryClient(srv.URL, &slept, testPolicy).do(context.Background(), http.MethodPost, "/x", map[string]string{"a": "b"}, nil)
	if err == nil || calls.Load() != 1 || len(slept) != 0 {
		t.Errorf("err = %v calls = %d sleeps = %v, want one failed call", err, calls.Load(), slept)
	}
}

func TestRetry_PostWithKeyRetriesAndSendsSameKey(t *testing.T) {
	var keys []string
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	var slept []time.Duration
	ctx := WithIdempotencyKey(context.Background(), "k-1")
	if err := retryClient(srv.URL, &slept, testPolicy).do(ctx, http.MethodPost, "/x", map[string]string{"a": "b"}, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(keys) != 2 || keys[0] != "k-1" || keys[1] != "k-1" {
		t.Errorf("keys = %v", keys)
	}
}

func TestRetry_BoundedByAttemptsAndWait(t *testing.T) {
	srv, calls := retryServer(t, http.StatusTooManyRequests, "1", 100)
	var slept []time.Duration
	err := retryClient(srv.URL, &slept, RetryPolicy{MaxRetries: 2, MaxWait: time.Minute, BaseDelay: time.Second}).do(context.Background(), http.MethodGet, "/x", nil, nil)
	if err == nil || calls.Load() != 3 {
		t.Errorf("attempts: err = %v calls = %d, want error after 3 calls", err, calls.Load())
	}
	slept, calls2 := nil, calls
	calls2.Store(0)
	err = retryClient(srv.URL, &slept, RetryPolicy{MaxRetries: 10, MaxWait: 1500 * time.Millisecond, BaseDelay: time.Second}).do(context.Background(), http.MethodGet, "/x", nil, nil)
	if err == nil || calls2.Load() > 2 {
		t.Errorf("wait budget: err = %v calls = %d, want stop once budget is spent", err, calls2.Load())
	}
}

func TestRetry_DisabledByDefault(t *testing.T) {
	srv, calls := retryServer(t, http.StatusTooManyRequests, "1", 5)
	if err := NewClient(srv.URL, "tok").do(context.Background(), http.MethodGet, "/x", nil, nil); err == nil || calls.Load() != 1 {
		t.Errorf("err = %v calls = %d, want a single call", err, calls.Load())
	}
}
