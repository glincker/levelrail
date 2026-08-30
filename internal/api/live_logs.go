package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// This file is the live-tailing counterpart to logs.go's
// handleQueryLogs: additive, not a replacement, per this task's own
// framing. handleQueryLogs answers "what did this app log in some past
// range," a request/response query over telemetry.DB's persisted store;
// handleLiveLogStream answers "what is this app logging right now," an
// SSE connection that stays open and pushes each line the instant
// LogCollector.StreamOne receives it from Docker (see
// internal/telemetry/broadcast.go's LogBroadcaster). Both read from the
// same resourceIDForApp(name) key (metrics.go), the identifier
// cmd/levelrail/main.go's log collector wiring already writes under.

// liveLogBackfillWindow bounds how far back handleLiveLogStream looks for
// context to show before switching to a live tail. Recent enough to be
// useful (what was this app doing right before this tab was opened,
// mirroring Coolify's own log panel opening on recent context instead of
// a blank screen), short enough that a container which has been logging
// for days doesn't turn "open the live log view" into "wait for the
// backfill query to scan a huge range" first. Paired with
// liveLogBackfillMaxLines below as a hard line-count cap regardless of
// window: QueryLogs itself has no LIMIT of its own, so a sufficiently
// chatty container could still produce an oversized burst within even
// this 5-minute window without that second cap.
const liveLogBackfillWindow = 5 * time.Minute

// liveLogBackfillMaxLines caps the backfill burst to the most recent N
// lines within liveLogBackfillWindow, oldest first: the same "last
// 100-200 lines" order of magnitude Coolify's own log panel opens on,
// picked as a number large enough to give real scrollback context
// without ever making the initial SSE burst itself the slow part of
// opening this view.
const liveLogBackfillMaxLines = 200

// handleLiveLogStream handles GET /api/v1/apps/{name}/logs/stream: an
// SSE stream of one app's container log output, live; see
// streamResourceLogs for the shared implementation and the
// backfill-to-live handoff this depends on.
func (rt *Router) handleLiveLogStream(w http.ResponseWriter, r *http.Request) {
	rt.streamResourceLogs(w, r, rt.lookupAppResource, "live log stream", "app")
}

// streamResourceLogs is the shared body behind handleLiveLogStream and
// handleLiveDatabaseLogStream (database_logs.go). 501 if either
// rt.telemetry (WithTelemetryQuerier) or rt.logBroadcaster
// (WithLogBroadcaster) isn't configured, since a live view needs both:
// the store for backfill, the broadcaster for everything after. opName
// and noun feed the log lines and 404 message so an app 404 reads
// "app not found" and a database 404 reads "database not found".
//
// The backfill-to-live handoff, and the gap it closes: Subscribe is
// called first, before the backfill query, and subscribeTime (the
// instant right after Subscribe returns) becomes an exclusive upper
// bound on what the backfill query is allowed to show (see
// trimBackfill). Getting this ordering backwards, querying first and
// subscribing second, would leave a real gap: any line
// LogCollector.StreamOne received in between those two steps would be
// published to no one (this handler wasn't subscribed yet) and might
// also not be in the store yet either (WriteLogBatch is batched, up to
// logBatchMaxWait behind real time), so it could be silently dropped,
// never shown at all. Subscribing first instead means every line from
// subscribeTime forward is guaranteed to arrive on the live channel,
// whether or not the backfill query's snapshot of the store happened to
// include it too; trimBackfill's cutoff at subscribeTime is what
// prevents that overlap from ever showing the same line twice. The
// honest remaining gap, deliberately not engineered away: a log line's
// own Timestamp (Docker's timestamp, or this collector's local clock
// fallback, see toLogEntry) could in principle be a few milliseconds
// behind the moment it was actually Published, so a line published in
// the same instant as subscribeTime could theoretically be timestamped
// just before it and get excluded from both the backfill cutoff and
// (having already been published) the live stream. This is a
// microsecond-scale race in practice, not a realistic operational
// concern, and is called out here rather than either quietly ignored or
// overclaimed as fully solved.
func (rt *Router) streamResourceLogs(w http.ResponseWriter, r *http.Request, lookup resourceLookup, opName, noun string) {
	if rt.telemetry == nil || rt.logBroadcaster == nil {
		writeError(w, http.StatusNotImplemented, "live log streaming is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	resourceID, found, err := lookup(r.Context(), name)
	if !found && err == nil {
		writeError(w, http.StatusNotFound, noun+" not found")
		return
	} else if err != nil {
		rt.logger.Error("api: "+opName+": load "+noun+" failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Subscribe before querying backfill: see this function's own doc
	// comment for why that ordering, not the reverse, is what closes the
	// gap between "read what's already stored" and "start receiving
	// what's published from here on."
	live, unsubscribe := rt.logBroadcaster.Subscribe(resourceID)
	defer unsubscribe()
	subscribeTime := time.Now()

	entries, err := rt.telemetry.QueryLogs(r.Context(), resourceID, subscribeTime.Add(-liveLogBackfillWindow), subscribeTime, "")
	if err != nil {
		// Unlike handleQueryLogs, a partial result here isn't treated as
		// a soft warning: this is a small, bounded backfill read, not a
		// federated multi-node query where "some sources answered" is a
		// meaningful, common outcome (see handleQueryLogs's own
		// comment on that distinction). A failure here almost certainly
		// means the store itself is unhealthy, worth surfacing as a real
		// error rather than silently opening a live view with no
		// context.
		rt.logger.Error("api: "+opName+": backfill query failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	flusher, ok := startSSE(w)
	if !ok {
		rt.logger.Error("api: "+opName+": response writer does not support flushing", slog.String("name", name))
		return
	}
	// See handleDeployLogStream's identical comment (deploy_attempts.go)
	// for why this unconditional comment line is written before anything
	// else: a zero-entries backfill (a freshly deployed, not-yet-noisy
	// app) would otherwise mean zero body bytes cross the wire until the
	// first live line, which some dev-proxy setups don't forward headers
	// for, leaving the frontend's connection badge stuck on
	// "Connecting..." even though the server is behaving correctly.
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for _, e := range trimBackfill(entries, subscribeTime, liveLogBackfillMaxLines) {
		writeSSEEvent(w, sseLogEvent{Line: e.Message, Stream: e.Stream})
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, chOpen := <-live:
			if !chOpen {
				// LogBroadcaster never closes a subscriber channel on its
				// own (unlike deploylog.Recorder's Finish, it has no
				// per-resource lifecycle to end, see that type's own doc
				// comment): a channel this handler is reading only stops
				// being written to once unsubscribe (deferred above) runs,
				// which happens after this select loop returns, not
				// before. So this branch is unreachable in practice today,
				// handled anyway rather than assumed impossible, matching
				// serveLiveDeployLog's identically-shaped defensive case
				// for the same reason: an incorrect assumption about
				// channel lifecycle is exactly the kind of bug that's
				// cheap to guard against and expensive to debug blind.
				return
			}
			writeSSEEvent(w, sseLogEvent{Line: entry.Message, Stream: entry.Stream})
			flusher.Flush()
		}
	}
}

// trimBackfill drops any entry at or after cutoff (see
// handleLiveLogStream's own doc comment on why that boundary, not the
// full backfill window, is what actually prevents a duplicate line) and,
// if more than maxLines remain, keeps only the most recent maxLines of
// them, preserving their original oldest-first order.
func trimBackfill(entries []telemetry.LogEntry, cutoff time.Time, maxLines int) []telemetry.LogEntry {
	out := make([]telemetry.LogEntry, 0, len(entries))
	for _, e := range entries {
		if e.Timestamp.Before(cutoff) {
			out = append(out, e)
		}
	}
	if len(out) > maxLines {
		out = out[len(out)-maxLines:]
	}
	return out
}
