package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// KindAdvisoryMatch fires while a running image matches a feed advisory at or above the severity floor.
const KindAdvisoryMatch Kind = "advisory_match"

// maxAdvisoryNoticeLines bounds one notice so a large feed cannot flood a channel.
const maxAdvisoryNoticeLines = 5

// AdvisoryHit is one running app image matching one advisory.
type AdvisoryHit struct {
	App        string
	AdvisoryID string
	Severity   string
	FixedIn    string
}

// AdvisorySource lists current matches at or above a severity floor; the runtime setting gate lives with the caller.
type AdvisorySource interface {
	AdvisoryHits(ctx context.Context, minSeverity string) ([]AdvisoryHit, error)
}

// SetAdvisoryWatch enables kind=advisory_match rules. enabled and floor are read every tick so the live settings apply.
func (e *Engine) SetAdvisoryWatch(src AdvisorySource, enabled func(context.Context) bool, floor func(context.Context) string) {
	e.advisories, e.advisoryEnabled, e.advisoryFloor = src, enabled, floor
}

// EvaluateAdvisoryMatch fires while any hit exists. With the feature disabled it reports no hits, so a firing rule resolves.
func EvaluateAdvisoryMatch(ctx context.Context, src AdvisorySource, enabled bool, floor string, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	var hits []AdvisoryHit
	if enabled {
		var err error
		if hits, err = src.AdvisoryHits(ctx, floor); err != nil {
			return r, "", fmt.Errorf("alerting: evaluate rule %q: advisory hits: %w", r.ID, err)
		}
	}
	v := float64(len(hits))
	next.LastValue = &v
	next = advanceState(next, r, len(hits) > 0, r.ForDuration, now)
	if len(hits) == 0 {
		return next, "", nil
	}
	lines := make([]string, 0, maxAdvisoryNoticeLines+1)
	for i, h := range hits {
		if i == maxAdvisoryNoticeLines {
			lines = append(lines, fmt.Sprintf("and %d more", len(hits)-i))
			break
		}
		s := fmt.Sprintf("%s: %s (%s)", cleanAlertText(h.App), cleanAlertText(h.AdvisoryID), cleanAlertText(h.Severity))
		if h.FixedIn != "" {
			s += ", fixed in " + cleanAlertText(h.FixedIn)
		}
		lines = append(lines, s)
	}
	return next, "advisory matches running images: " + strings.Join(lines, "; "), nil
}
