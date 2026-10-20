package main

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGracefulStopper blocks in GracefulStop until Stop, like grpc-go with a live stream.
type fakeGracefulStopper struct {
	block        bool
	stopCalled   atomic.Bool
	gracefulDone chan struct{}
}

func newFakeGracefulStopper(block bool) *fakeGracefulStopper {
	return &fakeGracefulStopper{block: block, gracefulDone: make(chan struct{})}
}

func (f *fakeGracefulStopper) GracefulStop() {
	if f.block {
		<-f.gracefulDone
	}
}

func (f *fakeGracefulStopper) Stop() {
	f.stopCalled.Store(true)
	close(f.gracefulDone)
}

func TestStopAgentGRPCServer_DrainsWithoutForcing(t *testing.T) {
	server := newFakeGracefulStopper(false)
	logger := slog.New(slog.DiscardHandler)

	done := make(chan struct{})
	go func() {
		stopAgentGRPCServer(server, logger, time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stopAgentGRPCServer did not return for an already-draining server")
	}
	if server.stopCalled.Load() {
		t.Error("Stop was called even though GracefulStop completed in time")
	}
}

// TestStopAgentGRPCServer_ForcesStopAfterTimeout checks that a stuck GracefulStop is forced to Stop after the timeout.
func TestStopAgentGRPCServer_ForcesStopAfterTimeout(t *testing.T) {
	server := newFakeGracefulStopper(true)
	logger := slog.New(slog.DiscardHandler)

	start := time.Now()
	done := make(chan struct{})
	go func() {
		stopAgentGRPCServer(server, logger, 50*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stopAgentGRPCServer did not return after its timeout elapsed, forced Stop never unblocked it")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("stopAgentGRPCServer took %v, want bounded close to the 50ms timeout", elapsed)
	}
	if !server.stopCalled.Load() {
		t.Error("Stop was never called for a server stuck in GracefulStop")
	}
}
