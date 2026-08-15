// Package deploylog is the glue between internal/build.ProgressEvent (the
// progress callback all three real deploy-attempt trigger paths already
// thread through internal/deploy.Pipeline.Deploy) and two real
// consumers: a persisted, replayable row in telemetry.db
// (internal/telemetry's deploy_logs table) and any currently-connected
// SSE viewer watching that same attempt live. Before this package
// existed, build.SlogProgress was the only real consumer: output went to
// slog and was gone the moment the process moved on, which
// docs-local/research/deploy-attempt-id-and-log-persistence.md's own
// framing (this product's core loop is unattended webhook deploys) calls
// out as the actual gap worth closing.
//
// One Recorder is shared across every trigger path in a running control
// plane (see cmd/levelrail/main.go's wiring): internal/api.Router's SSE
// route and internal/webhook.Handler's own webhook-triggered attempts
// both need to publish to and read from the exact same in-memory
// subscriber set, not two independent ones, or a viewer connected
// through the HTTP API would never see a line a webhook-triggered build
// produced.
package deploylog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// batchMaxLines mirrors internal/telemetry's own logBatchMaxLines: how
// many buffered lines trigger an immediate flush to the persisted store,
// so a long build's output doesn't accumulate into one enormous
// transaction. Unlike internal/telemetry.LogCollector, there is
// deliberately no time-based batchMaxWait companion here: a slow
// trickle of build output staying unflushed to disk for a few extra
// seconds only affects a *reconnecting* viewer (one who missed the live
// channel and falls back to the persisted store), and Finish's own
// unconditional final flush (called the moment Pipeline.Deploy returns,
// which is also the moment a build attempt can no longer produce more
// output) already bounds that staleness to "at most one attempt's worth
// of buffered lines," not an open-ended wait. A currently-connected live
// viewer never depends on the persisted store's flush timing at all: see
// Progress's own doc comment for why the live channel carries every line
// immediately, independent of the batching threshold below.
const batchMaxLines = 200

// subscriberBufferSize bounds how many live events a slow SSE writer can
// fall behind by before this package starts dropping events rather than
// blocking build progress on a stuck HTTP response. A dropped live event
// is not data loss: the line is still in the persisted store (or the
// in-memory lines-so-far buffer a fresh Snapshot call would return), only
// a live viewer's real-time view skips it. 256 is generous relative to
// the "dozens to low hundreds of lines" a real build produces
// (web/src/hooks/useDeployLogStream.ts's own sizing comment).
const subscriberBufferSize = 256

// LogStore is the narrow persistence surface Recorder needs.
// *telemetry.DB satisfies this structurally. nil is valid (see
// NewRecorder): without one configured, a Recorder still does live
// fan-out and in-memory lines-so-far snapshots correctly, it just never
// persists anything, so a viewer that connects after an attempt already
// finished (and was evicted from memory, see Finish) has nothing to
// replay. cmd/levelrail/main.go always configures a real one; this stays
// nil-tolerant purely so a router or test built without a telemetry
// store doesn't need a fake just to exist.
type LogStore interface {
	WriteDeployLogBatch(ctx context.Context, entries []telemetry.DeployLogEntry) error
}

// Event is one log line, the exact shape
// web/src/hooks/useDeployLogStream.ts's SSE contract prefers
// ({ "line": string, "stream": "stdout" | "stderr" }), fanned out live to
// a Snapshot subscriber and also what Snapshot's own lines-so-far slice
// is made of.
type Event struct {
	Line   string
	Stream string
}

// attemptState is one active (Start called, Finish not yet called)
// attempt's in-memory bookkeeping: every line seen so far (so a new
// Snapshot caller, connecting mid-build, gets full context rather than
// only what happens next), an unflushed tail buffer, and the live
// subscriber channels currently registered.
type attemptState struct {
	lines []Event                    // every line so far, for Snapshot's replay slice
	buf   []telemetry.DeployLogEntry // not yet written to store
	subs  []chan Event
}

// Recorder turns one attempt's build.ProgressEvent stream into both a
// persisted log and a live fan-out. See the package doc comment for why
// exactly one Recorder must be shared across every trigger path in a
// running control plane.
type Recorder struct {
	store  LogStore
	logger *slog.Logger

	mu     sync.Mutex
	active map[string]*attemptState
}

// NewRecorder builds a Recorder. store may be nil (see LogStore's own
// doc comment); logger defaults to slog.Default() if nil.
func NewRecorder(store LogStore, logger *slog.Logger) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{store: store, logger: logger, active: make(map[string]*attemptState)}
}

// Start registers attemptID as active, before any call to the func
// Progress returns and before any SSE viewer's Snapshot call can
// meaningfully attach to a live tail. A trigger handler must call this
// before invoking internal/deploy.Pipeline.Deploy with the progress func
// Progress(attemptID) returns; calling Progress before Start is a caller
// bug (the returned func would have nowhere to record into) and is
// treated as a no-op rather than a panic, the same "fail closed, not
// loudly, for an internal wiring mistake" choice this package makes
// throughout (see the store-error handling in the func Progress
// returns).
func (r *Recorder) Start(attemptID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active[attemptID] = &attemptState{}
}

// Progress returns a progress func bound to attemptID, suitable for
// passing straight to internal/deploy.Pipeline.Deploy in place of
// build.SlogProgress. Every event carrying output (ev.Log != "", i.e.
// not a bare step-lifecycle event like "step completed" or "step
// cached") becomes one Event: published immediately, synchronously, to
// every currently-registered live subscriber (never buffered or
// batched, so a live viewer sees output the moment it happens, not
// delayed by the persistence batching threshold below), and also
// appended to the attempt's unflushed persistence buffer, flushed to the
// store once that buffer reaches batchMaxLines. attemptID must have
// already been passed to Start; see Start's own doc comment for the
// no-op behavior otherwise.
//
// Safe for concurrent use: internal/build.Client.Build invokes the
// progress func from more than one goroutine at once (BuildKit's own
// solve-status relay and the docker-image-load phase run concurrently,
// see build.go's own errgroup wiring), so every access to attemptID's
// state here is under Recorder's single mutex.
func (r *Recorder) Progress(attemptID string) func(build.ProgressEvent) {
	return func(ev build.ProgressEvent) {
		if ev.Log == "" {
			return // step-lifecycle event, not output: nothing to record
		}
		stream := ev.Stream
		if stream == "" {
			stream = "stdout"
		}
		line := Event{Line: ev.Log, Stream: stream}

		r.mu.Lock()
		st, ok := r.active[attemptID]
		if !ok {
			r.mu.Unlock()
			return
		}
		st.lines = append(st.lines, line)
		st.buf = append(st.buf, telemetry.DeployLogEntry{
			AttemptID: attemptID, Stream: stream, Timestamp: time.Now(), Message: ev.Log,
		})
		for _, ch := range st.subs {
			select {
			case ch <- line:
			default:
				// Slow subscriber: drop rather than block build progress
				// on a stuck HTTP response writer. See
				// subscriberBufferSize's own doc comment.
			}
		}
		var toFlush []telemetry.DeployLogEntry
		if len(st.buf) >= batchMaxLines {
			toFlush = st.buf
			st.buf = nil
		}
		r.mu.Unlock()

		if toFlush != nil {
			r.flush(attemptID, toFlush)
		}
	}
}

func (r *Recorder) flush(attemptID string, entries []telemetry.DeployLogEntry) {
	if r.store == nil || len(entries) == 0 {
		return
	}
	// context.Background(), not a request-scoped context: a flush must
	// complete even if it's triggered by, e.g., an HTTP handler's
	// request context being cancelled mid-build (a viewer closing the
	// build-trigger request does not mean the build itself, or its log,
	// should stop being recorded). Matches internal/telemetry.LogCollector's
	// own shutdown-flush reasoning (logs.go's StreamOne).
	if err := r.store.WriteDeployLogBatch(context.Background(), entries); err != nil {
		r.logger.Error("deploylog: write batch failed",
			slog.String("attempt_id", attemptID), slog.String("error", err.Error()))
	}
}

// Finish flushes attemptID's remaining unflushed lines to the store,
// closes every live subscriber channel (the end-of-stream signal a
// Snapshot caller's read loop watches for, see
// internal/api/deploys.go's SSE handler), and evicts attemptID from
// memory. Must be called exactly once per attemptID, after
// internal/deploy.Pipeline.Deploy returns, success or failure; callers
// should defer this immediately after Start to guarantee it runs even if
// Deploy panics. A Snapshot call for attemptID after Finish returns
// false-ok, by design: the caller's job at that point is to fall back to
// the persisted store (QueryDeployLog) for a full replay, which Finish's
// own unconditional flush above guarantees is complete and available by
// the time Finish returns.
func (r *Recorder) Finish(ctx context.Context, attemptID string) {
	r.mu.Lock()
	st, ok := r.active[attemptID]
	if !ok {
		r.mu.Unlock()
		return
	}
	toFlush := st.buf
	subs := st.subs
	delete(r.active, attemptID)
	r.mu.Unlock()

	if r.store != nil && len(toFlush) > 0 {
		if err := r.store.WriteDeployLogBatch(ctx, toFlush); err != nil {
			r.logger.Error("deploylog: final flush failed",
				slog.String("attempt_id", attemptID), slog.String("error", err.Error()))
		}
	}
	for _, ch := range subs {
		close(ch)
	}
}

// Snapshot atomically returns every line recorded so far for attemptID
// plus a channel that receives every subsequent line, so a caller can
// never miss a line published between the snapshot and the channel
// registration (both happen under the same lock) and never see one
// twice. ok is false when attemptID isn't currently active: never
// Start()ed, or already Finish()ed and evicted. The caller's job in that
// case (see internal/api/deploys.go's SSE handler) is to fall back to
// the persisted store for a full replay.
//
// unsubscribe must be called exactly once, typically deferred, once the
// caller stops reading live (the SSE client disconnected, or the
// request context was cancelled): it removes the channel from
// attemptID's subscriber list so Progress stops trying to deliver to it.
// Safe to call after Finish has already closed the channel and evicted
// the attempt (a no-op in that case, attemptID's state is simply gone).
func (r *Recorder) Snapshot(attemptID string) (lines []Event, live <-chan Event, unsubscribe func(), ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.active[attemptID]
	if !ok {
		return nil, nil, func() {}, false
	}

	linesCopy := make([]Event, len(st.lines))
	copy(linesCopy, st.lines)

	ch := make(chan Event, subscriberBufferSize)
	st.subs = append(st.subs, ch)

	unsubscribe = func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		st2, ok := r.active[attemptID]
		if !ok {
			return
		}
		for i, c := range st2.subs {
			if c == ch {
				st2.subs = append(st2.subs[:i], st2.subs[i+1:]...)
				break
			}
		}
	}

	return linesCopy, ch, unsubscribe, true
}
