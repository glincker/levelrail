package provision

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := newHTTPClient(srv.URL, "secret-token")
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if want := "Bearer secret-token"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

func TestHTTPClient_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	c := newHTTPClient(srv.URL, "t")
	c.sleep = noopSleep // no real waiting in tests

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	if !out.OK {
		t.Errorf("out.OK = false, want true")
	}
}

func TestHTTPClient_GivesUpAfterMaxRetries(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	c := newHTTPClient(srv.URL, "t")
	c.sleep = noopSleep

	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil)
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if want := maxRetries + 1; calls != want {
		t.Errorf("calls = %d, want %d", calls, want)
	}
}

func TestHTTPClient_HonorsRetryAfterHeader(t *testing.T) {
	var delays []time.Duration
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := newHTTPClient(srv.URL, "t")
	c.sleep = func(_ context.Context, d time.Duration) error {
		delays = append(delays, d)
		return nil
	}

	if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if len(delays) != 1 || delays[0] != 7*time.Second {
		t.Errorf("delays = %v, want [7s]", delays)
	}
}

func TestHTTPClient_CapsRetryAfterDelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "9999")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	c := newHTTPClient(srv.URL, "t")
	var delays []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		delays = append(delays, d)
		return nil
	}

	_ = c.do(context.Background(), http.MethodGet, "/x", nil, nil)
	for _, d := range delays {
		if d > maxRetryDelay {
			t.Errorf("delay %v exceeds maxRetryDelay %v", d, maxRetryDelay)
		}
	}
}

func TestHTTPClient_RetryStopsOnContextCancel(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	c := newHTTPClient(srv.URL, "t")
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(sleepCtx context.Context, _ time.Duration) error {
		cancel()
		return sleepCtx.Err()
	}

	err := c.do(ctx, http.MethodGet, "/x", nil, nil)
	if err == nil {
		t.Fatal("expected an error when the context is cancelled mid-retry")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (retry loop must stop on cancellation)", calls)
	}
}

func noopSleep(context.Context, time.Duration) error { return nil }

func TestHTTPClient_NonRetryableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	t.Cleanup(srv.Close)

	c := newHTTPClient(srv.URL, "bad-token")
	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	var perr *ProviderError
	if !asProviderError(err, &perr) {
		t.Fatalf("error is not a *ProviderError: %v", err)
	}
	if perr.Status != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", perr.Status)
	}
}

func asProviderError(err error, target **ProviderError) bool {
	pe, ok := err.(*ProviderError)
	if ok {
		*target = pe
	}
	return ok
}
