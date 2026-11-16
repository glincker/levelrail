package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Webhook event types raised by the expiry sweep.
const (
	eventTokenExpiring = "tokens.expiring" //nolint:gosec // event type, not a credential
	eventTokenExpired  = "tokens.expired"  //nolint:gosec // event type, not a credential
	eventTokenRotated  = "tokens.rotated"  //nolint:gosec // event type, not a credential

	// reminderTick is how often the loop wakes; the settings interval gates the real work.
	reminderTick = 30 * time.Second
)

// reminderWindows reads the configured windows in days, widest first.
func (rt *Router) reminderWindows(ctx context.Context) []int {
	var out []int
	if rt.runtimeSettings != nil {
		for _, raw := range rt.runtimeSettings.List(ctx, runtimesettings.KeyTokensReminderWindows) {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
				out = append(out, n)
			}
		}
	}
	slices.SortFunc(out, func(a, b int) int { return b - a })
	return out
}

// reminderLevel is the narrowest window the time left falls in, in days;
// 0 means expired and -1 means no reminder is due yet.
func reminderLevel(left time.Duration, windows []int) int {
	if left <= 0 {
		return 0
	}
	level := -1
	for _, w := range windows {
		if left <= time.Duration(w)*24*time.Hour {
			level = w
		}
	}
	return level
}

// dueReminder reports whether level is newer than what was already sent for subject.
func dueReminder(sent map[string]int64, subject string, level int) bool {
	if level < 0 {
		return false
	}
	prev, ok := sent[subject]
	return !ok || int64(level) < prev
}

// RunTokenExpiryReminders raises one reminder per window per token, then
// once more when it has expired, and revokes rotated tokens after their grace.
func (rt *Router) RunTokenExpiryReminders(ctx context.Context) error {
	var last time.Time
	return footprint.Every(ctx, "token expiry reminders", reminderTick, rt.logger, func(ctx context.Context) error {
		interval := reminderTick
		if rt.runtimeSettings != nil {
			interval = rt.runtimeSettings.Duration(ctx, runtimesettings.KeyTokensReminderInterval)
		}
		now := time.Now()
		// A grace sweep error is returned so the loop records it in control plane health.
		graceErr := rt.sweepRotationGrace(ctx, now)
		rt.tokenState.setGraceFailing(graceErr != nil)
		if now.Sub(last) < interval {
			return graceErr
		}
		last = now
		return errors.Join(graceErr, rt.SweepTokenExpiry(ctx, now))
	})
}

// SweepTokenExpiry sends the reminders that are due at now.
func (rt *Router) SweepTokenExpiry(ctx context.Context, now time.Time) error {
	windows := rt.reminderWindows(ctx)
	if len(windows) == 0 {
		return nil
	}
	sent, err := rt.tokenStateLoad(ctx, tokenStateNotices)
	if err != nil {
		return err
	}
	pending, err := rt.tokenStateLoad(ctx, tokenStateRotation)
	if err != nil {
		return err
	}
	changed := map[string]int64{}
	var gone []string
	tokens, err := rt.libraryListTokens(ctx, "", true)
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, t := range tokens {
		live[t.ID] = true
		if t.RevokedAt != nil || t.ExpiresAt == nil || isPlatformToken(t) {
			continue
		}
		if _, rotating := pending[t.ID]; rotating {
			continue
		}
		level := reminderLevel(t.ExpiresAt.Sub(now), windows)
		if !dueReminder(sent, t.ID, level) {
			continue
		}
		typ := eventTokenExpiring
		if level == 0 {
			typ = eventTokenExpired
		}
		rt.EmitEvent(typ, map[string]any{
			"token_id": t.ID, "name": t.Name, "expires_at": t.ExpiresAt.UTC().Format(time.RFC3339),
			"window_days": level, "rotate_path": "/api/v1/auth/tokens/" + t.ID + "/rotate",
		})
		changed[t.ID] = int64(level)
	}
	for id := range sent {
		if strings.Contains(id, ":") {
			continue
		}
		if !live[id] {
			gone = append(gone, id)
		}
	}
	if len(changed) == 0 && len(gone) == 0 {
		return nil
	}
	return rt.tokenStateUpdate(ctx, tokenStateNotices, func(m map[string]int64) {
		for k, v := range changed {
			m[k] = v
		}
		for _, id := range gone {
			delete(m, id)
		}
	})
}

// sweepRotationGrace revokes old tokens whose grace has ended. It reports an
// unreadable state, since rotated tokens then stay valid past their grace.
func (rt *Router) sweepRotationGrace(ctx context.Context, now time.Time) error {
	pending, err := rt.tokenStateLoad(ctx, tokenStateRotation)
	if err != nil {
		rt.logger.Warn("api: rotation grace sweep cannot read pending revocations, rotated tokens stay valid until it can", slog.String("error", err.Error()))
		return fmt.Errorf("api: rotation grace sweep: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}
	var done []string
	for id, at := range pending {
		if at > now.Unix() {
			continue
		}
		if err := rt.libraryRevokeToken(ctx, id); err != nil && !isTokenGone(err) {
			rt.logger.Warn("api: revoke rotated token failed", slog.String("token_id", id), slog.String("error", err.Error()))
			continue
		}
		done = append(done, id)
	}
	if len(done) == 0 {
		return nil
	}
	if err := rt.tokenStateUpdate(ctx, tokenStateRotation, func(m map[string]int64) {
		for _, id := range done {
			delete(m, id)
		}
	}); err != nil {
		rt.logger.Warn("api: clear rotated token state failed", slog.String("error", err.Error()))
	}
	return nil
}

func isTokenGone(err error) bool { return errors.Is(err, store.ErrAPITokenNotFound) }
