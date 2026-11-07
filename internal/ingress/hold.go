package ingress

import (
	"fmt"
	"html"
	"sync"
	"time"
)

// HoldReason says why a held host has no backend. The empty value is the
// generic wording, used whenever the state must not be revealed.
type HoldReason string

// The reasons a hold page can state.
const (
	HoldReasonGeneric   HoldReason = ""
	HoldReasonDeploying HoldReason = "deploying"
	HoldReasonStopped   HoldReason = "stopped"
	HoldReasonUnhealthy HoldReason = "unhealthy"
)

const stoppedPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title></head>
<body style="font-family:sans-serif;text-align:center;padding:4rem 1rem">
<h1>%s</h1>
<p>%s</p>
</body>
</html>
`

// HoldResponse is the 503 a held host answers. A stopped app promises no
// recovery, so it carries no refresh and no Retry-After; every other reason
// retries on its own.
func (h Hardening) HoldResponse(reason HoldReason) StaticResponseHandler {
	switch reason {
	case HoldReasonStopped:
		title, msg := "Stopped", "This app is stopped."
		return StaticResponseHandler{
			Handler:    "static_response",
			StatusCode: 503,
			Body:       fmt.Sprintf(stoppedPageTemplate, html.EscapeString(title), html.EscapeString(title), html.EscapeString(msg)),
			Headers: map[string][]string{
				"Content-Type":  {"text/html; charset=utf-8"},
				"Cache-Control": {"no-store"},
			},
		}
	case HoldReasonDeploying:
		return h.UnavailableResponse("Updating", "This app is being deployed and will be back in a moment. This page will retry on its own.")
	case HoldReasonUnhealthy:
		return h.UnavailableResponse("Temporarily unavailable", "This app is not responding right now. This page will retry on its own.")
	default:
		return h.UnavailableResponse("Updating", "This site is being updated or was just stopped. If it is deploying it will be back in a moment. This page will retry on its own.")
	}
}

// HoldTracker remembers when each host last had a live backend, so a host that
// loses its backend mid-deploy keeps answering a friendly 503 (and keeps its
// certificate) for a bounded window instead of vanishing from the config.
// It must outlive per-pass controllers.
type HoldTracker struct {
	mu     sync.Mutex
	window time.Duration
	seen   map[string]time.Time
}

// NewHoldTracker builds a tracker; a window of zero or less disables holding.
func NewHoldTracker(window time.Duration) *HoldTracker {
	return &HoldTracker{window: window, seen: make(map[string]time.Time)}
}

// Routed records that host had a live backend at now.
func (t *HoldTracker) Routed(host string, now time.Time) {
	if t == nil || t.window <= 0 {
		return
	}
	t.mu.Lock()
	t.seen[host] = now
	t.mu.Unlock()
}

// Held reports whether host had a backend within the window, and forgets hosts
// whose window has lapsed.
func (t *HoldTracker) Held(host string, now time.Time) bool {
	if t == nil || t.window <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	at, ok := t.seen[host]
	if !ok {
		return false
	}
	if now.Sub(at) > t.window {
		delete(t.seen, host)
		return false
	}
	return true
}
