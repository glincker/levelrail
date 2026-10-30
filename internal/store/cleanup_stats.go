package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CleanupTable names a table the cleanup center can measure.
type CleanupTable string

// Measurable tables.
const (
	CleanupAuditLog          CleanupTable = "audit_log"
	CleanupEmailDeliveries   CleanupTable = "email_deliveries"
	CleanupWebhookDeliveries CleanupTable = "webhook_deliveries"
	CleanupEventDeliveries   CleanupTable = "event_webhook_deliveries"
)

// CleanupTableStats is the row count, the oldest row's time and an estimate of the table size.
type CleanupTableStats struct {
	Rows   int64
	Oldest time.Time
	Bytes  int64
}

type cleanupTableSpec struct {
	table, stats, countOlder string
	format                   func(time.Time) string
	parse                    func(string) (time.Time, error)
}

func parseRFC3339(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

func formatRFC3339(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func formatEmail(t time.Time) string { return t.UTC().Format(emailDeliveryTimeFormat) }

// Whole statements per table so no SQL is assembled from pieces.
var cleanupTables = map[CleanupTable]cleanupTableSpec{
	CleanupAuditLog: {
		table: "audit_log", format: FormatAuditTime, parse: parseRFC3339,
		stats:      `SELECT COUNT(*), COALESCE(MIN(created_at), '') FROM audit_log`,
		countOlder: `SELECT COUNT(*) FROM audit_log WHERE created_at < ?`,
	},
	CleanupEmailDeliveries: {
		table: "email_deliveries", format: formatEmail, parse: parseRFC3339,
		stats:      `SELECT COUNT(*), COALESCE(MIN(created_at), '') FROM email_deliveries`,
		countOlder: `SELECT COUNT(*) FROM email_deliveries WHERE created_at < ?`,
	},
	CleanupWebhookDeliveries: {
		table: "webhook_deliveries", format: formatRFC3339, parse: parseRFC3339,
		stats:      `SELECT COUNT(*), COALESCE(MIN(received_at), '') FROM webhook_deliveries`,
		countOlder: `SELECT COUNT(*) FROM webhook_deliveries WHERE received_at < ?`,
	},
	CleanupEventDeliveries: {
		table: "event_webhook_deliveries", format: FormatAuditTime, parse: parseRFC3339,
		stats:      `SELECT COUNT(*), COALESCE(MIN(updated_at), '') FROM event_webhook_deliveries WHERE state != 'pending'`,
		countOlder: `SELECT COUNT(*) FROM event_webhook_deliveries WHERE state != 'pending' AND updated_at < ?`,
	},
}

// CleanupStats measures t. Bytes is the table's on-disk size when SQLite reports it, else 0.
func (db *DB) CleanupStats(ctx context.Context, t CleanupTable) (CleanupTableStats, error) {
	spec, ok := cleanupTables[t]
	if !ok {
		return CleanupTableStats{}, fmt.Errorf("store: %q is not a cleanup table", t)
	}
	var st CleanupTableStats
	var oldest string
	if err := db.QueryRowContext(ctx, spec.stats).Scan(&st.Rows, &oldest); err != nil {
		return st, fmt.Errorf("store: measure %s: %w", spec.table, err)
	}
	if oldest != "" {
		st.Oldest, _ = spec.parse(oldest)
	}
	st.Bytes = db.tableBytes(ctx, spec.table)
	return st, nil
}

// CleanupCountOlder counts rows of t older than cutoff, which is what a delete with the same cutoff would remove.
func (db *DB) CleanupCountOlder(ctx context.Context, t CleanupTable, cutoff time.Time) (int64, error) {
	spec, ok := cleanupTables[t]
	if !ok {
		return 0, fmt.Errorf("store: %q is not a cleanup table", t)
	}
	var n int64
	if err := db.QueryRowContext(ctx, spec.countOlder, spec.format(cutoff)).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count old %s: %w", spec.table, err)
	}
	return n, nil
}

// CountAuditEntriesTiered counts what DeleteAuditEntriesTiered would remove for the same cutoffs.
func (db *DB) CountAuditEntriesTiered(ctx context.Context, cutoff, securityCutoff time.Time) (int64, error) {
	if securityCutoff.After(cutoff) {
		securityCutoff = cutoff
	}
	var n int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE created_at < ? AND (created_at < ? OR NOT `+securityAuditPredicate+`)`,
		FormatAuditTime(cutoff), FormatAuditTime(securityCutoff)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count audit entries to purge: %w", err)
	}
	return n, nil
}

// tableBytes reads the table's page usage from the dbstat virtual table; 0 when the build lacks it.
func (db *DB) tableBytes(ctx context.Context, table string) int64 {
	var n sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT SUM(pgsize) FROM dbstat WHERE name = ?`, table).Scan(&n); err != nil {
		return 0
	}
	return n.Int64
}
