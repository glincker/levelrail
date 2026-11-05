package store

import (
	"context"
	"fmt"
	"time"
)

// CertLedgerEntry is one remembered certificate attempt: kind is failed, issued or started.
type CertLedgerEntry struct {
	Domain string
	Kind   string
	At     time.Time
}

// SaveCertLedger records an attempt; a repeat of the same row is ignored.
func (db *DB) SaveCertLedger(ctx context.Context, e CertLedgerEntry) error {
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO cert_ledger (domain, kind, at) VALUES (?, ?, ?)`,
		e.Domain, e.Kind, e.At.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save cert ledger %q: %w", e.Domain, err)
	}
	return nil
}

// LoadCertLedger drops rows older than keep and returns the rest, oldest first.
func (db *DB) LoadCertLedger(ctx context.Context, now time.Time, keep time.Duration) ([]CertLedgerEntry, error) {
	cut := now.Add(-keep).UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `DELETE FROM cert_ledger WHERE at < ?`, cut); err != nil {
		return nil, fmt.Errorf("store: prune cert ledger: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT domain, kind, at FROM cert_ledger ORDER BY at`)
	if err != nil {
		return nil, fmt.Errorf("store: load cert ledger: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CertLedgerEntry
	for rows.Next() {
		var e CertLedgerEntry
		var at string
		if err := rows.Scan(&e.Domain, &e.Kind, &at); err != nil {
			return nil, fmt.Errorf("store: scan cert ledger: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			continue
		}
		e.At = t
		out = append(out, e)
	}
	return out, rows.Err()
}
