package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func hostPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestWaitReady_ImmediateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := WaitReady(ctx, srv.Client(), hostPort(t, srv), Config{Path: "/healthz", Interval: 50 * time.Millisecond, Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
}

func TestWaitReady_SucceedsAfterInitialFailures(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := WaitReady(ctx, srv.Client(), hostPort(t, srv), Config{Path: "/healthz", Interval: 20 * time.Millisecond, Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	if got := calls.Load(); got < 3 {
		t.Errorf("expected at least 3 attempts before success, got %d", got)
	}
}

func TestWaitReady_TimesOutWhenNeverHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := WaitReady(ctx, srv.Client(), hostPort(t, srv), Config{Path: "/healthz", Interval: 20 * time.Millisecond, Timeout: 500 * time.Millisecond})
	if err == nil {
		t.Fatal("WaitReady() error = nil, want a timeout error")
	}
}

func TestWaitReady_ConnectionRefused(t *testing.T) {
	// Nothing listening on this port: proves WaitReady handles a
	// connection-level failure (not just a bad status code) the same
	// way, by retrying rather than returning immediately.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := WaitReady(ctx, http.DefaultClient, "127.0.0.1:1", Config{Path: "/", Interval: 20 * time.Millisecond, Timeout: 30 * time.Millisecond})
	if err == nil {
		t.Fatal("WaitReady() error = nil, want an error for a port nothing listens on")
	}
}

func TestWaitReady_StatusCodeRanges(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantReady bool
	}{
		{name: "200 OK", status: http.StatusOK, wantReady: true},
		{name: "204 No Content", status: http.StatusNoContent, wantReady: true},
		{name: "299 edge of range", status: 299, wantReady: true},
		{name: "300 redirect, out of range", status: http.StatusMultipleChoices, wantReady: false},
		{name: "404 not found", status: http.StatusNotFound, wantReady: false},
		{name: "500 server error", status: http.StatusInternalServerError, wantReady: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			timeout := 300 * time.Millisecond
			if tt.wantReady {
				timeout = 2 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			err := WaitReady(ctx, srv.Client(), hostPort(t, srv), Config{Path: "/", Interval: 20 * time.Millisecond, Timeout: 100 * time.Millisecond})
			if tt.wantReady && err != nil {
				t.Errorf("status %d: WaitReady() error = %v, want nil", tt.status, err)
			}
			if !tt.wantReady && err == nil {
				t.Errorf("status %d: WaitReady() error = nil, want a timeout error", tt.status)
			}
		})
	}
}

func TestWaitReady_ZeroConfigUsesDefaults(t *testing.T) {
	// Interval and Timeout both zero must not busy-loop or hang forever;
	// defaultInterval/defaultTimeout apply.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := WaitReady(ctx, srv.Client(), hostPort(t, srv), Config{Path: "/"})
	if err != nil {
		t.Fatalf("WaitReady() with zero-value Config error = %v", err)
	}
}
