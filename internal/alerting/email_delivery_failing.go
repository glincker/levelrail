package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EmailDeliverySource reads the outbound email delivery log. *store.DB satisfies it.
type EmailDeliverySource interface {
	EmailDeliveryStats(ctx context.Context, since time.Time) (store.EmailDeliveryStats, error)
}

// Defaults for KindEmailDeliveryFailing: the look-back window and the failure count that must be exceeded.
const (
	EmailDeliveryFailingWindow       = 30 * time.Minute
	DefaultEmailDeliveryFailureFloor = 3
)

// EvaluateEmailDeliveryFailing fires while recent sends mostly fail: at least Threshold failures (default 3) in the window, outnumbering successes.
func EvaluateEmailDeliveryFailing(ctx context.Context, src EmailDeliverySource, r Rule, now time.Time) (Rule, string, error) {
	st, err := src.EmailDeliveryStats(ctx, now.Add(-EmailDeliveryFailingWindow))
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: email delivery stats: %w", r.ID, err)
	}
	floor := int(r.Threshold)
	if floor <= 0 {
		floor = DefaultEmailDeliveryFailureFloor
	}
	next := r
	next.LastEvaluatedAt = &now
	failing := st.Failed >= floor && st.Failed > st.Sent
	var notice string
	if failing {
		notice = fmt.Sprintf("%d of %d emails failed in the last %d minutes", st.Failed, st.Failed+st.Sent, int(EmailDeliveryFailingWindow.Minutes()))
		if st.LastFailure != nil && st.LastFailure.ErrorKind != "" {
			notice += " (latest: " + st.LastFailure.ErrorKind + ")"
		}
	}
	return advanceState(next, r, failing, r.ForDuration, now), notice, nil
}
