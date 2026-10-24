package cleanup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TokenSource deletes inactive API tokens. *store.DB satisfies it.
type TokenSource = janitor.TokenPurger

const (
	envTokenBatch    = "APP_TOKEN_CLEANUP_BATCH" //nolint:gosec // env var name
	defaultTokenStep = 200
)

// tokenTarget removes revoked tokens past the revoked window and expired ones past the expired window.
// The days argument is the revoked window; the expired window is its own runtime setting.
type tokenTarget struct {
	src      TokenSource
	settings Settings
	now      func() time.Time
}

func newTokenTarget(d BuiltinDeps) Target {
	return &tokenTarget{src: d.Tokens, settings: d.Settings, now: d.Now}
}

func (t *tokenTarget) Meta() Meta {
	m := baseMeta(TargetAPITokens, CategoryAuth, "Inactive API tokens",
		"Revoked and expired API tokens. Active tokens are never touched.", runtimesettings.KeyRevokedTokenDays)
	m.Presets = runtimesettings.TokenRetentionPresets
	return m
}

func tokenBatch() int {
	if v, ok := os.LookupEnv(envTokenBatch); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultTokenStep
}

func (t *tokenTarget) bounds(maxRows int64) (batch, maxBatches int) {
	batch = tokenBatch()
	maxBatches = 1
	if maxRows > 0 {
		maxBatches = int((maxRows + int64(batch) - 1) / int64(batch))
	}
	return batch, maxBatches
}

func (t *tokenTarget) sweep(ctx context.Context, revokedDays int, maxRows int64, dry bool) (int, bool, error) {
	batch, maxBatches := t.bounds(maxRows)
	expiredDays := t.settings.Int(ctx, runtimesettings.KeyExpiredTokenDays)
	total := 0
	for _, k := range []struct {
		kind store.TokenPurgeKind
		days int
	}{{store.TokenPurgeRevoked, revokedDays}, {store.TokenPurgeExpired, expiredDays}} {
		if k.days <= 0 {
			continue
		}
		n, err := janitor.PurgeTokens(ctx, t.src, k.kind, k.days, t.now(), batch, maxBatches, dry)
		total += n
		if err != nil {
			return total, false, err
		}
	}
	return total, total >= batch*maxBatches, nil
}

func (t *tokenTarget) Measure(ctx context.Context) (Usage, error) {
	batch, maxBatches := t.bounds(0)
	var total int
	for _, k := range []store.TokenPurgeKind{store.TokenPurgeRevoked, store.TokenPurgeExpired} {
		n, err := janitor.PurgeTokens(ctx, t.src, k, 0, t.now(), batch, maxBatches, true)
		if err != nil {
			return Usage{}, fmt.Errorf("cleanup: measure tokens: %w", err)
		}
		total += n
	}
	return Usage{Rows: int64(total), NoBytes: true}, nil
}

func (t *tokenTarget) Preview(ctx context.Context, days int) (Impact, error) {
	n, _, err := t.sweep(ctx, days, 0, true)
	return Impact{Rows: int64(n)}, err
}

func (t *tokenTarget) Run(ctx context.Context, days int, opts RunOptions) Result {
	n, more, err := t.sweep(ctx, days, opts.MaxRows, opts.DryRun)
	res := Result{Rows: int64(n), More: more}
	if err != nil {
		res.Err = fmt.Errorf("cleanup: %s: %w", TargetAPITokens, err)
	}
	return res
}
