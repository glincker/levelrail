package telemetry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeSink records every batch Send receives, safe for concurrent use
// since DrainForwarder.forwardOne calls Send from its own goroutine.
type fakeSink struct {
	mu      sync.Mutex
	batches [][]LogEntry
	sendErr error
}

func (f *fakeSink) Send(_ context.Context, entries []LogEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	batch := make([]LogEntry, len(entries))
	copy(batch, entries)
	f.batches = append(f.batches, batch)
	return nil
}

func (f *fakeSink) lineCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.batches {
		n += len(b)
	}
	return n
}

func TestBuildDrainSink(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DrainConfig
		wantErr bool
	}{
		{name: "http with target", cfg: DrainConfig{Type: DrainSinkHTTP, Target: "https://example.com/ingest"}},
		{name: "http without target is an error", cfg: DrainConfig{Type: DrainSinkHTTP, Target: ""}, wantErr: true},
		{name: "unknown type is an error", cfg: DrainConfig{Type: "carrier-pigeon", Target: "x"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink, err := buildDrainSink(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildDrainSink(%+v) error = nil, want an error", tt.cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildDrainSink(%+v) error = %v", tt.cfg, err)
			}
			if sink == nil {
				t.Fatalf("buildDrainSink(%+v) returned nil sink with no error", tt.cfg)
			}
		})
	}
}

// TestDrainForwarder_ForwardsPublishedLines proves a DrainForwarder is a
// pure additional consumer of LogBroadcaster: publishing through the
// broadcaster (the same call LogCollector.StreamOne makes) reaches a
// configured sink without needing anything else from the log ingestion
// pipeline.
func TestDrainForwarder_ForwardsPublishedLines(t *testing.T) {
	broadcaster := NewLogBroadcaster()
	sink := &fakeSink{}
	forwarder := NewDrainForwarder(broadcaster, nil)
	forwarder.buildSink = func(DrainConfig) (DrainSink, error) { return sink, nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	drainCtx, drainCancel := context.WithCancel(ctx)
	defer drainCancel()
	go forwarder.forwardOne(drainCtx, DrainConfig{ResourceID: "service:web", Type: DrainSinkHTTP, Target: "https://example.com", Enabled: true})

	// Give forwardOne's Subscribe call time to register before publishing,
	// the same "subscriber must exist before Publish" ordering
	// broadcast_test.go's own tests already rely on for LogBroadcaster.
	waitForSubscriber(t, broadcaster, "service:web")

	broadcaster.Publish(LogEntry{ResourceID: "service:web", Stream: "stdout", Message: "hello"})
	broadcaster.Publish(LogEntry{ResourceID: "service:web", Stream: "stdout", Message: "world"})
	broadcaster.Publish(LogEntry{ResourceID: "service:other", Stream: "stdout", Message: "not for us"})

	// Waited out via drainBatchMaxWait's own periodic flush, not
	// triggered by cancelling drainCtx: cancelling races forwardOne's
	// select against its own not-yet-drained channel (ctx.Done() and a
	// ready channel read can both be selectable at once, with no
	// ordering guarantee between them), so asserting on the *shutdown*
	// flush's line count would be asserting on a real race, not this
	// forwarder's actual delivery guarantee.
	waitForLineCount(t, sink, 2)
	drainCancel()

	if got := sink.lineCount(); got != 2 {
		t.Fatalf("sink received %d lines, want 2", got)
	}
}

func TestDrainForwarder_SinkBuildErrorDoesNotPanic(t *testing.T) {
	broadcaster := NewLogBroadcaster()
	forwarder := NewDrainForwarder(broadcaster, nil)
	forwarder.buildSink = func(DrainConfig) (DrainSink, error) { return nil, errors.New("boom") }

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	forwarder.forwardOne(ctx, DrainConfig{ResourceID: "service:web", Type: DrainSinkHTTP, Target: "https://example.com", Enabled: true})
}

func waitForSubscriber(t *testing.T, b *LogBroadcaster, resourceID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := len(b.subs[resourceID])
		b.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no subscriber registered for %q within timeout", resourceID)
}

func waitForLineCount(t *testing.T, sink *fakeSink, want int) {
	t.Helper()
	deadline := time.Now().Add(drainBatchMaxWait + time.Second)
	for time.Now().Before(deadline) {
		if sink.lineCount() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("sink received %d lines within timeout, want %d", sink.lineCount(), want)
}
