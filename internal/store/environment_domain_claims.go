package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func environmentDomainOwners(ctx context.Context, tx *sql.Tx, envID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT service_name FROM service_environment_domains WHERE environment_id = ?`, envID)
	if err != nil {
		return nil, fmt.Errorf("list environment domain owners: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scan environment domain owner: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// resyncServiceDomainClaims re-derives name's claims from its default set
// plus whatever environment sets remain.
func resyncServiceDomainClaims(ctx context.Context, tx *sql.Tx, name string) error {
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT domains FROM desired_services WHERE name = ?`, name).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("read domains of %q: %w", name, err)
	}
	var base []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &base); err != nil {
			return fmt.Errorf("decode domains of %q: %w", name, err)
		}
	}
	return claimServiceDomains(ctx, tx, name, base)
}
