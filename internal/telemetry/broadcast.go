package telemetry

import "sync"

// broadcastBufferSize bounds how many live log lines a slow SSE writer
// can fall behind by before this package starts dropping lines for that
// one subscriber, rather than blocking LogCollector.StreamOne's own
// Docker log stream consumption on a stuck HTTP response somewhere
// downstream. Mirrors internal/deploylog's subscriberBufferSize exactly:
// same reasoning (a dropped live line is not data loss, WriteLogBatch
// already persisted it or will shortly, only a live viewer's real-time
// view skips it) and the same order of magnitude (a live tail
// realistically shows a handful of lines per second from one container,
// not a per-frame stream).
const broadcastBufferSize = 256

// LogBroadcaster fans out every log line LogCollector.StreamOne receives
// to whichever SSE viewers are currently watching that line's
// ResourceID, live, the moment it arrives. It exists for the same reason
// internal/deploylog.Recorder exists for build/deploy output:
// persistence alone (WriteLogBatch, batched up to logBatchMaxWait behind
// real time) is fine for a historical search but not for a view labeled
// "live", which needs to feel instant.
//
// Deliberately simpler than deploylog.Recorder, in one specific way:
// there is no Start/Finish lifecycle here. A deploy attempt has a real
// bounded shape (started, eventually finishes, gets evicted), a
// container's ResourceID does not: a service can be redeployed,
// restarted, or have zero subscribers for most of its life, with no
// single moment that means "this resource is done, stop remembering
// it." So LogBroadcaster never eagerly allocates state for a resource,
// only when something actually calls Subscribe, and Unsubscribe cleans
// up empty entries rather than relying on a Finish call that has no
// natural trigger here. There is also no "lines so far" in-memory replay
// (contrast Recorder.Snapshot): that job belongs to the persisted store
// (QueryLogs) via internal/api's live-log handler, which backfills
// recent context from there before calling Subscribe, since only the
// store, not this in-memory broadcaster, survives a control plane
// restart.
//
// One instance is shared across every LogCollector.StreamOne goroutine
// (the publishing side, one per currently-running container) and every
// live-log SSE handler request (the subscribing side), the same
// "exactly one shared instance, wired once at startup" shape
// deploylog.Recorder's own package doc comment establishes for build
// logs: a viewer connected through the HTTP API must see the same lines
// the collector is receiving directly from Docker, which only works if
// both sides publish to and read from the same subscriber set.
type LogBroadcaster struct {
	mu   sync.Mutex
	subs map[string][]chan LogEntry // keyed by ResourceID
}

// NewLogBroadcaster builds an empty LogBroadcaster.
func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{subs: make(map[string][]chan LogEntry)}
}

// Publish delivers entry to every subscriber currently watching
// entry.ResourceID. Non-blocking by design: a subscriber whose channel
// is full (see broadcastBufferSize) is skipped for this entry rather
// than stalling the call, since Publish's real caller (StreamOne) is on
// the same goroutine that is also consuming Docker's own log stream and
// batching writes to the store, neither of which may block on an HTTP
// response writer somewhere downstream that a slow or stuck client left
// half-read.
func (b *LogBroadcaster) Publish(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[entry.ResourceID] {
		select {
		case ch <- entry:
		default:
			// Slow subscriber: drop rather than block. The entry is (or
			// will shortly be, see logBatchMaxWait) in the persisted
			// store regardless, so this is a live-view gap, not data
			// loss.
		}
	}
}

// Subscribe registers a new live listener for resourceID, returning a
// channel that receives every entry Published for that resourceID from
// this point forward, and an unsubscribe func the caller must call
// exactly once, typically deferred, once it stops reading (e.g. the SSE
// client disconnected). Safe to call for a resourceID with no active
// producer (the container hasn't started yet, or already stopped, or
// this control plane never ran a collector for it): Subscribe never
// fails, it may simply never receive anything, which is the correct
// behavior for a live tail opened against a currently quiet or
// not-yet-running app rather than an error condition.
func (b *LogBroadcaster) Subscribe(resourceID string) (ch <-chan LogEntry, unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	c := make(chan LogEntry, broadcastBufferSize)
	b.subs[resourceID] = append(b.subs[resourceID], c)

	unsubscribe = func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.subs[resourceID]
		for i, existing := range list {
			if existing == c {
				b.subs[resourceID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		// Delete the map entry entirely once its subscriber list is
		// empty, rather than leaving a zero-length slice behind.
		// Unlike deploylog.Recorder's active map, which is bounded by
		// however many deploy attempts are concurrently in flight and
		// explicitly evicted by Finish, resourceID here is effectively
		// unbounded over a control plane's lifetime: every app ever
		// deployed gets one. A map entry that only ever grows, never
		// shrinks, would be a slow but real memory leak across enough
		// redeploys and deletions on a long-running control plane.
		if len(b.subs[resourceID]) == 0 {
			delete(b.subs, resourceID)
		}
	}

	return c, unsubscribe
}
