package ingress

import (
	"net/http"
	"time"
)

// CutoverGap is the request damage the edge recorded around one cutover.
// GapMS spans the earliest start to the latest end of the failed or queued
// requests, so zero-downtime shows as 0 instead of a claim.
type CutoverGap struct {
	Measured bool      `json:"measured"`
	Failed   int       `json:"failed"`
	Queued   int       `json:"queued"`
	GapMS    int64     `json:"gap_ms"`
	FirstAt  time.Time `json:"first_at,omitempty"`
	LastAt   time.Time `json:"last_at,omitempty"`
	Window   int64     `json:"window_ms"`
	QueuedMS int64     `json:"queued_over_ms"`
}

// IsCutoverFailure reports whether a request failed because no upstream took it.
func IsCutoverFailure(r AccessRecord) bool {
	switch r.Status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return r.Upstream
}

// MeasureCutoverGap counts failed and queued (slower than queuedOver)
// requests among recs and the time they span. A record's start is its
// completion time minus its duration, so a request held across cutover counts
// from when it was first held.
func MeasureCutoverGap(recs []AccessRecord, window, queuedOver time.Duration) CutoverGap {
	g := CutoverGap{Measured: true, Window: window.Milliseconds(), QueuedMS: queuedOver.Milliseconds()}
	var first, last time.Time
	seen := map[string]bool{}
	for _, r := range recs {
		failed := IsCutoverFailure(r)
		queued := !failed && r.Millis >= float64(queuedOver.Milliseconds())
		if !failed && !queued {
			continue
		}
		if r.RequestID != "" {
			if seen[r.RequestID] {
				continue
			}
			seen[r.RequestID] = true
		}
		if failed {
			g.Failed++
		} else {
			g.Queued++
		}
		start := r.Time.Add(-time.Duration(r.Millis * float64(time.Millisecond)))
		if first.IsZero() || start.Before(first) {
			first = start
		}
		if r.Time.After(last) {
			last = r.Time
		}
	}
	if g.Failed+g.Queued > 0 {
		g.FirstAt, g.LastAt = first, last
		g.GapMS = last.Sub(first).Milliseconds()
	}
	return g
}
