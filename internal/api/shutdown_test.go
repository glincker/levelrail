package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBeginShutdownEndsSSEButNotOrdinaryRequests(t *testing.T) {
	rt := &Router{}
	streamEnded := make(chan struct{})
	plainRelease := make(chan struct{})
	plainCtxErr := make(chan error, 1)
	h := rt.streamDrainMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
			close(streamEnded)
			return
		}
		<-plainRelease
		plainCtxErr <- r.Context().Err()
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	go func() {
		resp, err := http.Get(srv.URL + "/stream")
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
	}()
	plainDone := make(chan struct{})
	go func() {
		defer close(plainDone)
		if resp, err := http.Get(srv.URL + "/plain"); err == nil {
			_ = resp.Body.Close()
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		rt.shutdown.mu.Lock()
		n := len(rt.shutdown.streams)
		rt.shutdown.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream was not registered, have %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := rt.BeginShutdown(); got != 1 {
		t.Fatalf("BeginShutdown closed %d streams, want 1", got)
	}
	select {
	case <-streamEnded:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not return after BeginShutdown")
	}
	close(plainRelease)
	<-plainDone
	if err := <-plainCtxErr; err != nil {
		t.Fatalf("ordinary request was cancelled: %v", err)
	}
}

func TestWaitDeploys(t *testing.T) {
	rt := &Router{}
	rt.shutdown.deploys.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if rt.WaitDeploys(ctx) {
		t.Fatal("WaitDeploys reported done with a deploy running")
	}
	rt.shutdown.deploys.Done()
	if !rt.WaitDeploys(context.Background()) {
		t.Fatal("WaitDeploys did not return after the deploy finished")
	}
}
