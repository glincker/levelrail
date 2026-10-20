package store

import (
	"context"
	"fmt"
	"time"
)

// ManagedDNSRecord is a DNS record Levelrail created for a domain, so only
// records it owns are ever removed (migrations/0418).
type ManagedDNSRecord struct {
	Domain     string
	Provider   string
	Zone       string
	Name       string
	RecordType string
	Value      string
	AppName    string
	CreatedAt  time.Time
}

// SaveManagedDNSRecord inserts or replaces the record tracked for
// (domain, record type).
func (db *DB) SaveManagedDNSRecord(ctx context.Context, r ManagedDNSRecord) error {
	created := r.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO managed_dns_records (domain, provider, zone, name, record_type, value, app_name, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (domain, record_type) DO UPDATE SET
			provider = excluded.provider, zone = excluded.zone, name = excluded.name,
			value = excluded.value, app_name = excluded.app_name
	`, r.Domain, r.Provider, r.Zone, r.Name, r.RecordType, r.Value, r.AppName, created.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save managed dns record: %w", err)
	}
	return nil
}

// ListManagedDNSRecords returns the records tracked for domain.
func (db *DB) ListManagedDNSRecords(ctx context.Context, domain string) ([]ManagedDNSRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, provider, zone, name, record_type, value, app_name, created_at
		FROM managed_dns_records WHERE domain = ? ORDER BY record_type
	`, domain)
	if err != nil {
		return nil, fmt.Errorf("store: list managed dns records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ManagedDNSRecord
	for rows.Next() {
		var r ManagedDNSRecord
		var created string
		if err := rows.Scan(&r.Domain, &r.Provider, &r.Zone, &r.Name, &r.RecordType, &r.Value, &r.AppName, &created); err != nil {
			return nil, fmt.Errorf("store: scan managed dns record: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate managed dns records: %w", err)
	}
	return out, nil
}

// DeleteManagedDNSRecord forgets the tracked record for (domain, type).
func (db *DB) DeleteManagedDNSRecord(ctx context.Context, domain, recordType string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM managed_dns_records WHERE domain = ? AND record_type = ?`, domain, recordType); err != nil {
		return fmt.Errorf("store: delete managed dns record: %w", err)
	}
	return nil
}
