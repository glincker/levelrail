package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/prcommand"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// refusalAuditWindow admits one refusal audit row per key per window and
// counts the rest, so a comment loop costs one row per window, not per comment.
type refusalAuditWindow struct {
	mu        sync.Mutex
	window    time.Duration
	maxKeys   int
	seen      map[string]*refusalWindowEntry
	lastSweep time.Time
}

type refusalWindowEntry struct {
	start      time.Time
	suppressed int
	last       refusalRecord
}

// refusalRecord is what a summary row needs to describe the latest refusal.
type refusalRecord struct {
	name   string
	ev     webhook.IssueCommentEvent
	cmd    prcommand.Command
	reason string
}

// refusalSummary carries suppressed refusals from a window that closed
// without a later refusal for the same key.
type refusalSummary struct {
	rec        refusalRecord
	suppressed int
}

func newRefusalAuditWindow(window time.Duration, maxKeys int) *refusalAuditWindow {
	return &refusalAuditWindow{window: window, maxKeys: maxKeys, seen: make(map[string]*refusalWindowEntry)}
}

// admit reports whether key may write a row now, how many refusals were
// suppressed for key since its previous row, and summaries for other keys
// whose windows closed with suppressed refusals.
func (w *refusalAuditWindow) admit(key string, rec refusalRecord, now time.Time) (bool, int, []refusalSummary) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e := w.seen[key]
	if e != nil && now.Sub(e.start) < w.window {
		e.suppressed++
		e.last = rec
		return false, 0, nil
	}
	carried := 0
	if e != nil {
		carried = e.suppressed
		delete(w.seen, key)
	}
	var closed []refusalSummary
	// Sweep at most once per window unless full, so a quiet login's count still surfaces.
	if len(w.seen) >= w.maxKeys || now.Sub(w.lastSweep) >= w.window {
		w.lastSweep = now
		for k, old := range w.seen {
			if now.Sub(old.start) < w.window {
				continue
			}
			if old.suppressed > 0 {
				closed = append(closed, refusalSummary{rec: old.last, suppressed: old.suppressed})
			}
			delete(w.seen, k)
		}
	}
	// Full of live windows: drop the row rather than grow without bound.
	if len(w.seen) >= w.maxKeys {
		return false, 0, closed
	}
	w.seen[key] = &refusalWindowEntry{start: now}
	return true, carried, closed
}

// auditPRRefusalWindowed writes a refusal row through pc's per app, login and
// reason window. The caller must already hold the comment id claim.
func (rt *Router) auditPRRefusalWindowed(ctx context.Context, pc *prCommentState, name string, ev webhook.IssueCommentEvent, cmd prcommand.Command, reason string) {
	if pc == nil || pc.refused == nil {
		return
	}
	rec := refusalRecord{name: name, ev: ev, cmd: cmd, reason: reason}
	ok, suppressed, closed := pc.refused.admit(name+"|"+strings.ToLower(ev.Commenter)+"|"+reason, rec, pc.now())
	for _, c := range closed {
		rt.auditPRCommandRefused(ctx, c.rec.name, c.rec.ev, c.rec.cmd, c.rec.reason, prRefusalWindowSummaryNote, c.suppressed)
	}
	if !ok {
		return
	}
	rt.auditPRCommandRefused(ctx, name, ev, cmd, reason, "", suppressed)
}

// prRefusalWindowSummaryNote marks a row that only reports suppressed refusals.
const prRefusalWindowSummaryNote = "window closed with suppressed refusals"
