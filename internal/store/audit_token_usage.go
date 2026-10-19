package store

import (
	"context"
	"fmt"
	"time"
)

// TokenAbilityUsage is how often one token exercised one route ability since a cutoff.
type TokenAbilityUsage struct {
	TokenID  string
	Ability  string
	Requests int
	LastAt   string
}

// TokenAbilityUsageSince counts successful audited token requests per token and ability.
func (db *DB) TokenAbilityUsageSince(ctx context.Context, since time.Time) ([]TokenAbilityUsage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT actor_id, ability, COUNT(*), MAX(created_at)
		FROM audit_log
		WHERE actor_type = 'token' AND ability != '' AND created_at >= ? AND status_code < 400
		GROUP BY actor_id, ability`, FormatAuditTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: token ability usage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TokenAbilityUsage
	for rows.Next() {
		var u TokenAbilityUsage
		if err := rows.Scan(&u.TokenID, &u.Ability, &u.Requests, &u.LastAt); err != nil {
			return nil, fmt.Errorf("store: scan token ability usage: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate token ability usage: %w", err)
	}
	return out, nil
}
