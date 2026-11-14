package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// retryClient points at a server that answers statuses in order, with a fake clock.
func retryClient(t *testing.T, statuses []int, header http.Header) (*Client, *atomic.Int32, *[]time.Duration) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		st := statuses[min(i, len(statuses)-1)]
		for k, v := range header {
			w.Header()[k] = v
		}
		w.WriteHeader(st)
		if st == 200 {
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "tok").withRetry(4, 30*time.Second)
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	return c, &n, &slept
}

func TestRetryThenSuccess(t *testing.T) {
	c, n, slept := retryClient(t, []int{503, 429, 200}, nil)
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 3 || len(*slept) != 2 {
		t.Fatalf("calls %d sleeps %v", n.Load(), *slept)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	c, n, _ := retryClient(t, []int{400, 200}, nil)
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); err == nil || n.Load() != 1 {
		t.Fatalf("a 400 must not be retried: calls %d err %v", n.Load(), err)
	}
}

func TestPostRetriedOnlyWithKey(t *testing.T) {
	c, n, _ := retryClient(t, []int{500, 200}, nil)
	if err := c.do(context.Background(), http.MethodPost, "/x", map[string]string{"a": "b"}, nil); err == nil || n.Load() != 1 {
		t.Fatalf("POST without a key must not be retried: calls %d", n.Load())
	}
	c, n, _ = retryClient(t, []int{500, 200}, nil)
	if err := c.doKeyed(context.Background(), http.MethodPost, "/x", "key-1", map[string]string{"a": "b"}, nil); err != nil || n.Load() != 2 {
		t.Fatalf("POST with a key must be retried: calls %d err %v", n.Load(), err)
	}
}

func TestRetryAfterHonoredAndCapped(t *testing.T) {
	c, _, slept := retryClient(t, []int{429, 200}, http.Header{"Retry-After": {"7"}})
	if err := c.do(context.Background(), http.MethodDelete, "/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 7*time.Second {
		t.Fatalf("Retry-After not honored: %v", *slept)
	}
	c, _, slept = retryClient(t, []int{429, 200}, http.Header{"Retry-After": {"120"}})
	c.withRetry(4, 10*time.Second)
	_ = c.do(context.Background(), http.MethodGet, "/x", nil, nil)
	if (*slept)[0] != 10*time.Second {
		t.Fatalf("wait must be capped: %v", *slept)
	}
}

func TestRetryGivesUpAfterMax(t *testing.T) {
	c, n, _ := retryClient(t, []int{503}, nil)
	c.withRetry(2, time.Second)
	if err := c.do(context.Background(), http.MethodPut, "/x", nil, nil); err == nil || n.Load() != 3 {
		t.Fatalf("want 3 attempts and an error, got %d %v", n.Load(), err)
	}
}
