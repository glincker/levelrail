package janitor

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TokenPurger deletes inactive API tokens. *store.DB satisfies it.
type TokenPurger interface {
	PurgeInactiveAPITokens(ctx context.Context, kind store.TokenPurgeKind, cutoff time.Time, limit int, dryRun bool) (int, error)
}

// TokenWindows are the retention windows in days; 0 keeps that kind forever.
type TokenWindows struct{ Revoked, Expired int }

const (
	defaultTokenBatch      = 200
	defaultTokenMaxBatches = 25
)

// PurgeTokens deletes tokens of kind older than days, in batches of batch up to
// maxBatches, and returns the total. With dryRun it counts one capped sweep.
func PurgeTokens(ctx context.Context, p TokenPurger, kind store.TokenPurgeKind, days int, now time.Time, batch, maxBatches int, dryRun bool) (int, error) {
	if batch < 1 {
		batch = defaultTokenBatch
	}
	if maxBatches < 1 {
		maxBatches = defaultTokenMaxBatches
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	if dryRun {
		n, err := p.PurgeInactiveAPITokens(ctx, kind, cutoff, batch*maxBatches, true)
		if err != nil {
			return 0, fmt.Errorf("janitor: count %s tokens: %w", kind, err)
		}
		return n, nil
	}
	total := 0
	for i := 0; i < maxBatches; i++ {
		n, err := p.PurgeInactiveAPITokens(ctx, kind, cutoff, batch, false)
		total += n
		if err != nil {
			return total, fmt.Errorf("janitor: purge %s tokens: %w", kind, err)
		}
		if n < batch {
			break
		}
	}
	return total, nil
}

// tokens removes revoked and expired API tokens past their windows. A window
// of 0 turns that kind off, so a scheduled pass never touches it.
func (e *Engine) tokens(ctx context.Context, dry bool, trigger string, now time.Time) StageResult {
	if e.TokenWindows == nil {
		return skipped(StageTokens, "token retention is not configured")
	}
	w := e.TokenWindows(ctx)
	res := StageResult{Stage: StageTokens, Status: StatusRan, Items: []Item{}}
	if w.Revoked <= 0 && w.Expired <= 0 {
		res.Reason = "token retention is set to keep forever"
		return res
	}
	act := ActionWouldRemove
	if !dry {
		act = ActionRemoved
	}
	total := 0
	for _, k := range []struct {
		kind store.TokenPurgeKind
		days int
	}{{store.TokenPurgeRevoked, w.Revoked}, {store.TokenPurgeExpired, w.Expired}} {
		if k.days <= 0 {
			continue
		}
		n, err := PurgeTokens(ctx, e.Store, k.kind, k.days, now, e.TokenBatch, e.TokenMaxBatches, dry)
		if n > 0 {
			total += n
			res.Items = append(res.Items, Item{Name: string(k.kind) + " tokens", Detail: fmt.Sprintf("%d %s tokens older than %d days", n, k.kind, k.days), Count: n, Action: act})
		}
		if err != nil {
			res.Status, res.Error = StatusFailed, err.Error()
			break
		}
	}
	if total > 0 && !dry && e.Audit != nil {
		e.Audit(ctx, AuditTokensCleanup, fmt.Sprintf(`{"deleted":%d,"trigger":%q}`, total, trigger))
	}
	return res
}
