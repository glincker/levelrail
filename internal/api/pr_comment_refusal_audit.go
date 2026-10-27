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
	mu      sync.Mutex
	window  time.Duration
	maxKeys int
	seen    map[string]*refusalWindowEntry
}

type refusalWindowEntry struct {
	start      time.Time
	suppressed int
}

func newRefusalAuditWindow(window time.Duration, maxKeys int) *refusalAuditWindow {
	return &refusalAuditWindow{window: window, maxKeys: maxKeys, seen: make(map[string]*refusalWindowEntry)}
}

// admit reports whether key may write a row now, and how many refusals were
// suppressed for key since its previous row.
func (w *refusalAuditWindow) admit(key string, now time.Time) (bool, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e := w.seen[key]
	if e != nil && now.Sub(e.start) < w.window {
		e.suppressed++
		return false, 0
	}
	if e == nil && len(w.seen) >= w.maxKeys {
		for k, old := range w.seen {
			if now.Sub(old.start) >= w.window {
				delete(w.seen, k)
			}
		}
		// Full of live windows: drop the row rather than grow without bound.
		if len(w.seen) >= w.maxKeys {
			return false, 0
		}
	}
	carried := 0
	if e != nil {
		carried = e.suppressed
	}
	w.seen[key] = &refusalWindowEntry{start: now}
	return true, carried
}

// auditPRRefusalWindowed writes a refusal row through pc's per app and login
// window. The caller must already hold the comment id claim.
func (rt *Router) auditPRRefusalWindowed(ctx context.Context, pc *prCommentState, name string, ev webhook.IssueCommentEvent, cmd prcommand.Command, reason string) {
	if pc == nil || pc.refused == nil {
		return
	}
	ok, suppressed := pc.refused.admit(name+"|"+strings.ToLower(ev.Commenter), pc.now())
	if !ok {
		return
	}
	rt.auditPRCommandRefused(ctx, name, ev, cmd, reason, "", suppressed)
}
