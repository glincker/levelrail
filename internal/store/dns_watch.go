package store

import (
	"context"
	"fmt"
	"time"
)

// DNSWatch is one domain the DNS watcher follows. Zero times are stored as empty text.
type DNSWatch struct {
	Domain      string
	App         string
	State       string
	Since       time.Time
	LastChecked time.Time
	NextCheck   time.Time
	Attempts    int
	StalledAt   time.Time
	Stopped     bool
	NotifiedAt  time.Time
}

func dnsWatchTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseDNSWatchTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// SaveDNSWatch inserts or replaces the row for w.Domain.
func (db *DB) SaveDNSWatch(ctx context.Context, w DNSWatch) error {
	stopped := 0
	if w.Stopped {
		stopped = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO dns_watch (domain, app, state, since, last_checked, next_check, attempts, stalled_at, stopped, notified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET app = excluded.app, state = excluded.state, since = excluded.since,
			last_checked = excluded.last_checked, next_check = excluded.next_check, attempts = excluded.attempts,
			stalled_at = excluded.stalled_at, stopped = excluded.stopped, notified_at = excluded.notified_at`,
		w.Domain, w.App, w.State, dnsWatchTime(w.Since), dnsWatchTime(w.LastChecked), dnsWatchTime(w.NextCheck),
		w.Attempts, dnsWatchTime(w.StalledAt), stopped, dnsWatchTime(w.NotifiedAt))
	if err != nil {
		return fmt.Errorf("store: save dns watch %q: %w", w.Domain, err)
	}
	return nil
}

// DeleteDNSWatch removes the row for domain; a missing row is not an error.
func (db *DB) DeleteDNSWatch(ctx context.Context, domain string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM dns_watch WHERE domain = ?`, domain); err != nil {
		return fmt.Errorf("store: delete dns watch %q: %w", domain, err)
	}
	return nil
}

// ListDNSWatch returns every watched domain ordered by domain.
func (db *DB) ListDNSWatch(ctx context.Context) ([]DNSWatch, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain, app, state, since, last_checked, next_check, attempts, stalled_at, stopped, notified_at
		FROM dns_watch ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list dns watch: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DNSWatch
	for rows.Next() {
		var w DNSWatch
		var since, last, next, stalled, notified string
		var stopped int
		if err := rows.Scan(&w.Domain, &w.App, &w.State, &since, &last, &next, &w.Attempts, &stalled, &stopped, &notified); err != nil {
			return nil, fmt.Errorf("store: scan dns watch: %w", err)
		}
		w.Since, w.LastChecked, w.NextCheck = parseDNSWatchTime(since), parseDNSWatchTime(last), parseDNSWatchTime(next)
		w.StalledAt, w.NotifiedAt, w.Stopped = parseDNSWatchTime(stalled), parseDNSWatchTime(notified), stopped != 0
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list dns watch: %w", err)
	}
	return out, nil
}
