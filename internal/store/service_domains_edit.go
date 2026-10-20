package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// EditServiceDomains rewrites only name's domain list, reading and writing
// it in one transaction so a concurrent edit to any other setting is never
// overwritten. edit returns the new list; its error aborts the change.
// Returns ErrServiceNotFound, or *ErrDomainTaken via errors.As.
func (db *DB) EditServiceDomains(ctx context.Context, name string, edit func(current []string) ([]string, error)) (domains []string, changed bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("store: edit domains for %q: begin transaction: %w", name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	var raw string
	err = tx.QueryRowContext(ctx, `SELECT domains FROM desired_services WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrServiceNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: edit domains for %q: read: %w", name, err)
	}
	var current []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &current); err != nil {
			return nil, false, fmt.Errorf("store: edit domains for %q: decode: %w", name, err)
		}
	}
	next, err := edit(slices.Clone(current))
	if err != nil {
		return nil, false, err
	}
	next = nonNilSlice(next)
	if slices.Equal(current, next) {
		return next, false, nil
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return nil, false, fmt.Errorf("store: edit domains for %q: encode: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET domains = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(encoded), name); err != nil {
		return nil, false, fmt.Errorf("store: edit domains for %q: write: %w", name, err)
	}
	if err := claimServiceDomains(ctx, tx, name, next); err != nil {
		return nil, false, fmt.Errorf("store: edit domains for %q: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("store: edit domains for %q: commit: %w", name, err)
	}
	return next, true, nil
}
