package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envTokenHygieneInterval     = "APP_TOKEN_HYGIENE_SWEEP_INTERVAL" //nolint:gosec // env var name, not a credential
	defaultTokenHygieneInterval = time.Hour
	tokenHygieneAuditPath       = "/api/v1/security/token-hygiene/"
	hygieneDay                  = 24 * time.Hour
)

// hygieneClock throttles the unused-token sweep, which rides the faster
// sign-in expiry ticker.
type hygieneClock struct {
	mu   sync.Mutex
	last time.Time
}

func (c *hygieneClock) due(now time.Time, every time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.last.IsZero() && now.Sub(c.last) < every {
		return false
	}
	c.last = now
	return true
}

// tokenLifetimeProblem enforces max_token_lifetime_days on a new token.
func (rt *Router) tokenLifetimeProblem(ctx context.Context, expiresInDays int) (string, error) {
	p, _, err := rt.effectiveSecurityPolicy(ctx)
	if err != nil {
		return "", err
	}
	if p.MaxTokenLifetimeDays > 0 && (expiresInDays == 0 || expiresInDays > p.MaxTokenLifetimeDays) {
		return fmt.Sprintf("the security policy limits API tokens to %d days: set expires_in_days from 1 to %d", p.MaxTokenLifetimeDays, p.MaxTokenLifetimeDays), nil
	}
	return "", nil
}

func tokenLastActive(t store.APIToken) time.Time {
	if t.LastUsedAt != nil && t.LastUsedAt.After(t.CreatedAt) {
		return *t.LastUsedAt
	}
	return t.CreatedAt
}

// hygieneStep is what the sweep does to one token.
type hygieneStep int

const (
	hygieneNone hygieneStep = iota
	hygieneNotice
	hygieneDisable
	hygieneClear
)

// decideHygiene is the sweep's rule for one owned, live token: a notice once
// it has been idle disableDays, a revoke only after grace has passed since
// that notice, and the notice is dropped if the token is used again.
func decideHygiene(t store.APIToken, notice *store.TokenHygieneNotice, disableDays int, grace time.Duration, now time.Time) hygieneStep {
	if disableDays <= 0 {
		return hygieneNone
	}
	last := tokenLastActive(t)
	idle := now.Sub(last) >= time.Duration(disableDays)*hygieneDay
	switch {
	case notice == nil && idle:
		return hygieneNotice
	case notice == nil, !notice.DisabledAt.IsZero():
		return hygieneNone
	case last.After(notice.NoticedAt):
		return hygieneClear
	case idle && now.Sub(notice.NoticedAt) >= grace:
		return hygieneDisable
	}
	return hygieneNone
}

// SweepTokenHygiene runs the unused-token policy and prunes expired sign-in
// alert links and long-unseen browsers. System tokens (no owner) are never
// touched: the platform itself may depend on them.
func (rt *Router) SweepTokenHygiene(ctx context.Context, now time.Time) error {
	if rt.security == nil || !rt.sec.hygiene.due(now, envDuration(envTokenHygieneInterval, defaultTokenHygieneInterval)) {
		return nil
	}
	if err := rt.security.PruneSignInAlertTokens(ctx, now); err != nil {
		return err
	}
	if err := rt.security.PruneKnownBrowsers(ctx, now.Add(-envDuration(envKnownBrowserKeep, defaultKnownBrowserKeep))); err != nil {
		return err
	}
	p, _, err := rt.effectiveSecurityPolicy(ctx)
	if err != nil || p.DisableUnusedDays <= 0 || rt.authLib.tokens == nil {
		return err
	}
	recs, err := rt.authLib.tokens.ListTokens(ctx, "")
	if err != nil {
		return fmt.Errorf("api: token hygiene: list tokens: %w", err)
	}
	notices, err := rt.security.ListTokenHygieneNotices(ctx)
	if err != nil {
		return err
	}
	byID := make(map[string]*store.TokenHygieneNotice, len(notices))
	for i := range notices {
		byID[notices[i].TokenID] = &notices[i]
	}
	grace := tokenHygieneGrace()
	for _, rec := range recs {
		t := *libraryTokenToStore(rec)
		if t.OwnerUserID == "" || t.RevokedAt != nil || (t.ExpiresAt != nil && !t.ExpiresAt.After(now)) {
			continue
		}
		rt.applyHygieneStep(ctx, t, decideHygiene(t, byID[t.ID], p.DisableUnusedDays, grace, now), p.DisableUnusedDays, grace, now)
	}
	rt.sec.posture.invalidate()
	return nil
}

func (rt *Router) applyHygieneStep(ctx context.Context, t store.APIToken, step hygieneStep, days int, grace time.Duration, now time.Time) {
	actor := signInActor{kind: auditActorSystem, id: t.OwnerUserID, name: "Token hygiene"}
	switch step {
	case hygieneNotice:
		if err := rt.security.SaveTokenHygieneNotice(ctx, store.TokenHygieneNotice{TokenID: t.ID, OwnerID: t.OwnerUserID, TokenName: t.Name, NoticedAt: now}); err != nil {
			rt.logger.Warn("api: token hygiene: save notice failed", slog.String("token_id", t.ID), slog.String("error", err.Error()))
			return
		}
		rt.auditSignIn(ctx, nil, actor, store.AuditActionTokenUnusedNotice, tokenHygieneAuditPath+t.ID, http.StatusOK)
		rt.sendSecurityNotice(fmt.Sprintf("Security: API token %q has not been used for %d days and will be disabled in %d days unless it is used.",
			safeSignInText(t.Name), days, int(grace/hygieneDay)))
	case hygieneDisable:
		if err := rt.libraryRevokeToken(ctx, t.ID); err != nil {
			rt.logger.Warn("api: token hygiene: revoke failed", slog.String("token_id", t.ID), slog.String("error", err.Error()))
			return
		}
		if err := rt.security.MarkTokenHygieneDisabled(ctx, t.ID, now); err != nil {
			rt.logger.Warn("api: token hygiene: mark disabled failed", slog.String("token_id", t.ID), slog.String("error", err.Error()))
		}
		rt.auditSignIn(ctx, nil, actor, store.AuditActionTokenUnusedDisabled, tokenHygieneAuditPath+t.ID, http.StatusOK)
		rt.sendSecurityNotice(fmt.Sprintf("Security: API token %q was disabled after %d days without use.", safeSignInText(t.Name), days))
	case hygieneClear:
		if err := rt.security.DeleteTokenHygieneNotice(ctx, t.ID); err != nil {
			rt.logger.Warn("api: token hygiene: clear notice failed", slog.String("token_id", t.ID), slog.String("error", err.Error()))
		}
	}
}

// tokenHygieneItems lists tokens waiting to be disabled: the caller's own,
// or every one for an admin.
func (rt *Router) tokenHygieneItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if rt.security == nil {
		return nil
	}
	admin := hasAbility(abilities, AbilityRoot)
	callerID, _ := rt.currentSessionUserID(r)
	notices, err := rt.security.ListTokenHygieneNotices(r.Context())
	if err != nil {
		rt.logger.Warn("api: attention feed: list token notices failed", slog.String("error", err.Error()))
		return nil
	}
	grace := tokenHygieneGrace()
	var items []attention.Item
	for _, n := range notices {
		if !n.DisabledAt.IsZero() || (!admin && (callerID == "" || n.OwnerID != callerID)) {
			continue
		}
		at := n.NoticedAt.Add(grace)
		it := feedItem(attention.Warning, attention.KindTokenUnused, n.TokenName,
			"unused, disabled after "+at.UTC().Format("2006-01-02")+" unless it is used",
			map[string]string{"name": n.TokenName, "at": at.UTC().Format(time.RFC3339), "days_left": strconv.Itoa(max(0, int(at.Sub(now)/hygieneDay)))})
		it.ID += ":" + n.TokenID
		items = append(items, it)
	}
	return items
}
