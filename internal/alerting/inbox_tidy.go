package alerting

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// Env vars for the inbox sweep. A zero duration disables that step; an invalid one falls back to the default.
const (
	EnvInboxAutoArchiveAfter = "APP_INBOX_AUTO_ARCHIVE_RESOLVED_AFTER"
	EnvInboxRetention        = "APP_INBOX_ARCHIVE_RETENTION"
	EnvInboxTidyInterval     = "APP_INBOX_TIDY_INTERVAL"

	defaultInboxAutoArchiveAfter = 30 * time.Minute
	defaultInboxRetention        = 30 * 24 * time.Hour
	defaultInboxTidyInterval     = 10 * time.Minute
)

// TidyConfig sets when resolved items archive themselves and when archived ones are pruned.
type TidyConfig struct {
	ArchiveResolvedAfter time.Duration
	Retention            time.Duration
	Interval             time.Duration
}

func envDuration(getenv func(string) string, key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return def
	}
	return d
}

// TidyConfigFromEnv reads the sweep settings, falling back to the defaults.
func TidyConfigFromEnv(getenv func(string) string) TidyConfig {
	c := TidyConfig{
		ArchiveResolvedAfter: envDuration(getenv, EnvInboxAutoArchiveAfter, defaultInboxAutoArchiveAfter),
		Retention:            envDuration(getenv, EnvInboxRetention, defaultInboxRetention),
		Interval:             envDuration(getenv, EnvInboxTidyInterval, defaultInboxTidyInterval),
	}
	if c.Interval <= 0 {
		c.Interval = defaultInboxTidyInterval
	}
	return c
}

// TidyRunner archives resolved alert items and prunes old archived ones for every principal that uses the inbox.
type TidyRunner struct {
	DB     *DB
	Cfg    TidyConfig
	Logger *slog.Logger
	Now    func() time.Time
}

// TidyResult counts what one sweep did.
type TidyResult struct {
	Archived int
	Pruned   int
}

// RunOnce performs one sweep.
func (r *TidyRunner) RunOnce(ctx context.Context) (TidyResult, error) {
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	var res TidyResult
	principals, err := r.DB.inboxPrincipals(ctx)
	if err != nil {
		return res, err
	}
	for _, p := range principals {
		if r.Cfg.ArchiveResolvedAfter > 0 {
			sel := InboxSelector{Scope: InboxScopeResolved, OlderThan: r.Cfg.ArchiveResolvedAfter, Now: now}
			n, err := r.DB.ClearInbox(ctx, p, InboxActionArchive, sel, false)
			if err != nil {
				return res, fmt.Errorf("alerting: tidy archive for %s: %w", p, err)
			}
			res.Archived += n
		}
		if r.Cfg.Retention > 0 {
			n, err := r.DB.pruneArchived(ctx, p, now.Add(-r.Cfg.Retention), now)
			if err != nil {
				return res, fmt.Errorf("alerting: tidy prune for %s: %w", p, err)
			}
			res.Pruned += n
		}
	}
	if _, err := r.DB.ExecContext(ctx, `DELETE FROM inbox_deleted WHERE NOT EXISTS (SELECT 1 FROM alert_history h WHERE h.id = inbox_deleted.item_id)`); err != nil {
		return res, fmt.Errorf("alerting: tidy orphan deletes: %w", err)
	}
	return res, nil
}

func (db *DB) inboxPrincipals(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT principal FROM inbox_prefs UNION SELECT principal FROM inbox_read_state
		UNION SELECT principal FROM inbox_read UNION SELECT principal FROM inbox_archive`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list inbox principals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("alerting: scan inbox principal: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// pruneArchived removes stored alert items archived before cutoff. Derived items are never pruned,
// they would reappear while their source still exists.
func (db *DB) pruneArchived(ctx context.Context, principal string, cutoff, now time.Time) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.item_id FROM inbox_archive a JOIN alert_history h ON h.id = a.item_id
		WHERE a.principal = ? AND COALESCE(NULLIF(a.archived_at, ''), h.at) < ?`, principal, fmtTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("alerting: select prunable inbox items: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("alerting: scan prunable inbox item: %w", err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	err = db.inTx(ctx, "prune inbox", func(tx *sql.Tx) error {
		if err := deleteTx(ctx, tx, principal, ids, false, now); err != nil {
			return err
		}
		return archiveTx(ctx, tx, principal, ids, false, now)
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// Run sweeps every Cfg.Interval until ctx is done.
func (r *TidyRunner) Run(ctx context.Context) error {
	loop := footprint.Track("inbox tidy", r.Cfg.Interval)
	defer loop.Active()()
	t := time.NewTicker(r.Cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if res, err := r.RunOnce(ctx); err != nil {
				r.log().Error("alerting: inbox tidy failed", slog.String("error", err.Error()))
			} else if res.Archived+res.Pruned > 0 {
				r.log().Info("alerting: inbox tidied", slog.Int("archived", res.Archived), slog.Int("pruned", res.Pruned))
			}
		}
	}
}

func (r *TidyRunner) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
