package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheck_SingleAttemptPerCall(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "200 OK", status: http.StatusOK},
		{name: "204 No Content", status: http.StatusNoContent},
		{name: "404 not found", status: http.StatusNotFound, wantErr: true},
		{name: "503 unavailable", status: http.StatusServiceUnavailable, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			err := Check(context.Background(), srv.Client(), hostPort(t, srv), Config{Path: "/livez", Timeout: time.Second})
			if tt.wantErr != (err != nil) {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("attempts = %d, want exactly 1: Check must never loop, its caller's loop is the loop", got)
			}
		})
	}
}

// TestCheck_HangingHandlerFailsOnTimeout covers the case liveness exists
// for: the container is running and still accepting connections, but
// wedged, so it never answers.
func TestCheck_HangingHandlerFailsOnTimeout(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(block)

	start := time.Now()
	err := Check(context.Background(), srv.Client(), hostPort(t, srv), Config{Path: "/livez", Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("Check() error = nil, want a timeout error against a hung handler")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Check() took %v, want it bounded by Timeout", elapsed)
	}
}

func TestCheck_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	addr := hostPort(t, srv)
	srv.Close()

	if err := Check(context.Background(), http.DefaultClient, addr, Config{Path: "/livez", Timeout: 200 * time.Millisecond}); err == nil {
		t.Error("Check() error = nil, want a connection error")
	}
}

// TestCheck_ZeroTimeoutUsesDefault pins that a spec with no timeout set
// still bounds the attempt rather than hanging on a wedged container.
func TestCheck_ZeroTimeoutUsesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Check(context.Background(), srv.Client(), hostPort(t, srv), Config{Path: "/"}); err != nil {
		t.Fatalf("Check() with zero-value Config error = %v", err)
	}
}
