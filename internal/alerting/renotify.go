package alerting

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AckSource reports whether anyone has read the latest firing notice for a rule in the Inbox.
type AckSource interface {
	LatestFiredAcknowledged(ctx context.Context, ruleID string) (bool, error)
}

// LatestFiredAcknowledged reports whether the newest fired entry of a rule was read by any principal.
func (db *DB) LatestFiredAcknowledged(ctx context.Context, ruleID string) (bool, error) {
	var read int
	err := db.QueryRowContext(ctx, `
		SELECT CASE WHEN EXISTS (SELECT 1 FROM inbox_read r WHERE r.history_id = h.id)
			OR EXISTS (SELECT 1 FROM inbox_read_state s WHERE s.read_before IS NOT NULL AND h.at <= s.read_before)
			THEN 1 ELSE 0 END
		FROM alert_history h WHERE h.rule_id = ? AND h.event = 'fired'
		ORDER BY h.at DESC LIMIT 1`, ruleID).Scan(&read)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return read == 1, nil
}

// SetRenotify repeats a still-firing alert every interval until it is read in the Inbox; interval is read per tick so a setting change applies live.
func (e *Engine) SetRenotify(interval func(context.Context) time.Duration, acks AckSource) {
	e.renotifyEvery, e.acks = interval, acks
}

// ownsReminder is true for kinds that already repeat on their own schedule.
func ownsReminder(k Kind) bool {
	return k == KindCertExpiry || k == KindBudgetThreshold || k == KindSLOBurn
}

func (e *Engine) renotifyMark(id string, now time.Time, keep bool) {
	e.certMu.Lock()
	defer e.certMu.Unlock()
	if e.renotified == nil {
		e.renotified = map[string]time.Time{}
	}
	if keep {
		e.renotified[id] = now
		return
	}
	delete(e.renotified, id)
}

// renotifyDue allows one repeat per interval; the first sighting after a restart only starts the clock.
func (e *Engine) renotifyDue(ctx context.Context, r Rule, now time.Time) bool {
	if e.renotifyEvery == nil || ownsReminder(r.Kind) {
		return false
	}
	every := e.renotifyEvery(ctx)
	if every <= 0 {
		return false
	}
	e.certMu.Lock()
	last, seen := e.renotified[r.ID]
	if e.renotified == nil {
		e.renotified = map[string]time.Time{}
	}
	if !seen || now.Sub(last) < every {
		if !seen {
			e.renotified[r.ID] = now
		}
		e.certMu.Unlock()
		return false
	}
	e.renotified[r.ID] = now
	e.certMu.Unlock()
	if e.acks != nil {
		if ok, err := e.acks.LatestFiredAcknowledged(ctx, r.ID); err == nil && ok {
			return false
		}
	}
	return true
}
