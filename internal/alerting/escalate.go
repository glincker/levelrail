package alerting

import (
	"context"
	"log/slog"
	"time"
)

// escalationChannelSource is the optional RuleStore surface that loads the second channel.
type escalationChannelSource interface {
	GetNotificationChannel(ctx context.Context, id string) (*NotificationChannel, error)
}

func (e *Engine) escalationMark(id string, since time.Time, keep bool) {
	e.certMu.Lock()
	defer e.certMu.Unlock()
	if !keep {
		delete(e.escalated, id)
		return
	}
	if e.escalated == nil {
		e.escalated = map[string]time.Time{}
	}
	e.escalated[id] = since
}

// escalationDue is true once per firing episode, after EscalateAfter of firing with no Inbox read.
func (e *Engine) escalationDue(ctx context.Context, r Rule, now time.Time) bool {
	if r.EscalateChannelID == "" || r.EscalateAfter <= 0 || r.FiringSince == nil || ownsReminder(r.Kind) {
		return false
	}
	if now.Sub(*r.FiringSince) < r.EscalateAfter {
		return false
	}
	e.certMu.Lock()
	done, seen := e.escalated[r.ID]
	e.certMu.Unlock()
	if seen && done.Equal(*r.FiringSince) {
		return false
	}
	// Reading the alert settles the episode, so the second channel is never paged for it.
	e.escalationMark(r.ID, *r.FiringSince, true)
	if e.acks != nil {
		if ok, err := e.acks.LatestFiredAcknowledged(ctx, r.ID); err == nil && ok {
			return false
		}
	}
	return true
}

// escalate sends the firing alert to the rule's second channel; a missing or disabled channel is logged and skipped.
func (e *Engine) escalate(ctx context.Context, r Rule, ev Event) {
	if e.noise != nil {
		if _, muted := e.noise.silencedBy(ctx, e.noise.describe(ctx, r).ac, e.now()); muted {
			return
		}
	}
	src, ok := e.rules.(escalationChannelSource)
	if !ok {
		return
	}
	ch, err := src.GetNotificationChannel(ctx, r.EscalateChannelID)
	if err != nil || !ch.Enabled {
		e.logger.Warn("alerting: escalation channel unavailable", slog.String("rule_id", r.ID), slog.String("channel_id", r.EscalateChannelID))
		return
	}
	r.ChannelID, r.NotifyURL, r.NotifyKind = ch.ID, ch.NotifyURL, ch.Kind
	ev.Rule = r
	_ = e.sendEvent(ctx, ev)
}
