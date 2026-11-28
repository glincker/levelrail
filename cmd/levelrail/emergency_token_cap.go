package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/glincker/levelrail/internal/authengine"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// revokeOldestEmergencyTokens keeps room for one more live emergency token by
// revoking the oldest ones beyond maxLive. maxLive 0 means no limit.
func revokeOldestEmergencyTokens(ctx context.Context, db *store.DB, eng *authengine.Engine, maxLive int, now time.Time) (int, error) {
	if maxLive <= 0 {
		return 0, nil
	}
	recs, err := eng.ListTokens(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("list emergency tokens: %w", err)
	}
	var live []authengine.TokenRecord
	for _, r := range recs {
		if r.Name == emergencyTokenName && r.RevokedAt == nil && (r.ExpiresAt == nil || r.ExpiresAt.After(now)) {
			live = append(live, r)
		}
	}
	excess := len(live) - (maxLive - 1)
	if excess <= 0 {
		return 0, nil
	}
	slices.SortFunc(live, func(a, b authengine.TokenRecord) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for _, r := range live[:excess] {
		if err := eng.RevokeToken(ctx, r.EngineID); err != nil {
			return 0, fmt.Errorf("revoke emergency token: %w", err)
		}
		if r.ID != r.EngineID {
			if err := db.RevokeAPIToken(ctx, r.ID); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
				return 0, fmt.Errorf("revoke emergency token mirror: %w", err)
			}
		}
	}
	return excess, nil
}

func emergencyMaxLive(ctx context.Context, db *store.DB) int {
	return runtimesettings.New(db, nil).Int(ctx, runtimesettings.KeyTokensEmergencyMaxLive)
}
