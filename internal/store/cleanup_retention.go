package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RetentionTable names a history table the cleanup center bounds by age.
type RetentionTable string

// Tables bounded by RetentionDeleteOlder.
const (
	RetentionAuthSessions    RetentionTable = "theauth_sessions"
	RetentionAuthDeviceCodes RetentionTable = "theauth_device_codes"
	RetentionAuthMagicLinks  RetentionTable = "theauth_magic_links"
	RetentionAIChatMessages  RetentionTable = "ai_chat_messages"
	RetentionAIChatSessions  RetentionTable = "ai_chat_sessions"
	RetentionAIUsage         RetentionTable = "ai_usage"
	RetentionRestoreHistory  RetentionTable = "restore_history"
	RetentionPITRRestore     RetentionTable = "pitr_restore_history"
	RetentionBackupDrills    RetentionTable = "backup_drills"
	RetentionProbeAttempts   RetentionTable = "probe_attempts"
	RetentionAppEvents       RetentionTable = "app_events"
	RetentionAppSchedule     RetentionTable = "app_schedule_history"
	RetentionCanaryOutcomes  RetentionTable = "canary_outcomes"
	RetentionUsageCostDaily  RetentionTable = "usage_cost_daily"
	RetentionStatusIncidents RetentionTable = "status_incidents"

	RetentionMCPOAuthRequests RetentionTable = "mcp_oauth_requests"
	RetentionMCPOAuthRefresh  RetentionTable = "mcp_oauth_refresh"
	RetentionMCPOAuthTokens   RetentionTable = "mcp_oauth_tokens" //nolint:gosec // table name, not a credential
	RetentionMCPOAuthGrants   RetentionTable = "mcp_oauth_grants"
)

const (
	usageCostDayLayout        = "2006-01-02"
	goTimeStringLayout        = "2006-01-02 15:04:05.999999999 -0700 MST"
	retentionDefaultBatchSize = 1000
)

// retentionSpec holds whole statements per table so no SQL is assembled from pieces. Each count and delete
// takes one bound cutoff and removes only rows that are finished and strictly older than it.
type retentionSpec struct {
	stats, countOlder, deleteOlder string
	arg                            func(time.Time) any
	parse                          func(string) (time.Time, error)
}

func argMicro(t time.Time) any { return t.UTC().UnixMicro() }

func parseMicro(s string) (time.Time, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse unix micro %q: %w", s, err)
	}
	return time.UnixMicro(n).UTC(), nil
}

func argUnix(t time.Time) any { return t.UTC().Unix() }

func parseUnix(s string) (time.Time, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse unix seconds %q: %w", s, err)
	}
	return time.Unix(n, 0).UTC(), nil
}

func argTime(t time.Time) any { return t.UTC() }

// parseDriverTime reads a time.Time the sqlite driver stored as text.
func parseDriverTime(s string) (time.Time, error) {
	if i := strings.Index(s, " m="); i >= 0 {
		s = s[:i]
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(goTimeStringLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse stored time %q: %w", s, err)
	}
	return t.UTC(), nil
}

func argRFC3339(t time.Time) any { return t.UTC().Format(time.RFC3339) }

func argRFC3339Nano(t time.Time) any { return t.UTC().Format(time.RFC3339Nano) }

func argEventTime(t time.Time) any { return t.UTC().Format(appEventTimeFormat) }

func argDay(t time.Time) any { return t.UTC().Format(usageCostDayLayout) }

func parseDay(s string) (time.Time, error) { return time.Parse(usageCostDayLayout, s) }

var retentionTables = map[RetentionTable]retentionSpec{
	RetentionAuthSessions: {
		arg: argMicro, parse: parseMicro,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(expires_at) AS TEXT), '') FROM theauth_sessions`,
		countOlder:  `SELECT COUNT(*) FROM theauth_sessions WHERE expires_at < ?`,
		deleteOlder: `DELETE FROM theauth_sessions WHERE rowid IN (SELECT rowid FROM theauth_sessions WHERE expires_at < ? LIMIT ?)`,
	},
	RetentionAuthDeviceCodes: {
		arg: argMicro, parse: parseMicro,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(expires_at) AS TEXT), '') FROM theauth_device_codes`,
		countOlder:  `SELECT COUNT(*) FROM theauth_device_codes WHERE expires_at < ?`,
		deleteOlder: `DELETE FROM theauth_device_codes WHERE rowid IN (SELECT rowid FROM theauth_device_codes WHERE expires_at < ? LIMIT ?)`,
	},
	RetentionAuthMagicLinks: {
		arg: argMicro, parse: parseMicro,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(expires_at) AS TEXT), '') FROM theauth_magic_links`,
		countOlder:  `SELECT COUNT(*) FROM theauth_magic_links WHERE expires_at < ?`,
		deleteOlder: `DELETE FROM theauth_magic_links WHERE rowid IN (SELECT rowid FROM theauth_magic_links WHERE expires_at < ? LIMIT ?)`,
	},
	// A message goes only when its whole chat has been idle past the cutoff, so a chat that continues keeps a replayable history.
	RetentionAIChatMessages: {
		arg: argTime, parse: parseDriverTime,
		stats:      `SELECT COUNT(*), COALESCE(CAST(MIN(created_at) AS TEXT), '') FROM ai_chat_messages`,
		countOlder: `SELECT COUNT(*) FROM ai_chat_messages WHERE session_id IN (SELECT id FROM ai_chat_sessions WHERE updated_at < ?)`,
		deleteOlder: `DELETE FROM ai_chat_messages WHERE rowid IN (SELECT rowid FROM ai_chat_messages
			WHERE session_id IN (SELECT id FROM ai_chat_sessions WHERE updated_at < ?) LIMIT ?)`,
	},
	RetentionAIChatSessions: {
		arg: argTime, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(updated_at) AS TEXT), '') FROM ai_chat_sessions`,
		countOlder:  `SELECT COUNT(*) FROM ai_chat_sessions WHERE updated_at < ?`,
		deleteOlder: `DELETE FROM ai_chat_sessions WHERE rowid IN (SELECT rowid FROM ai_chat_sessions WHERE updated_at < ? LIMIT ?)`,
	},
	RetentionAIUsage: {
		arg: argRFC3339, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(created_at), '') FROM ai_usage`,
		countOlder:  `SELECT COUNT(*) FROM ai_usage WHERE created_at < ?`,
		deleteOlder: `DELETE FROM ai_usage WHERE id IN (SELECT id FROM ai_usage WHERE created_at < ? LIMIT ?)`,
	},
	RetentionRestoreHistory: {
		arg: argRFC3339, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(started_at), '') FROM restore_history`,
		countOlder:  `SELECT COUNT(*) FROM restore_history WHERE status != 'running' AND started_at < ?`,
		deleteOlder: `DELETE FROM restore_history WHERE id IN (SELECT id FROM restore_history WHERE status != 'running' AND started_at < ? LIMIT ?)`,
	},
	RetentionPITRRestore: {
		arg: argRFC3339, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(started_at), '') FROM pitr_restore_history`,
		countOlder:  `SELECT COUNT(*) FROM pitr_restore_history WHERE status != 'running' AND started_at < ?`,
		deleteOlder: `DELETE FROM pitr_restore_history WHERE id IN (SELECT id FROM pitr_restore_history WHERE status != 'running' AND started_at < ? LIMIT ?)`,
	},
	RetentionBackupDrills: {
		arg: argRFC3339, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(started_at), '') FROM backup_drills`,
		countOlder:  `SELECT COUNT(*) FROM backup_drills WHERE status != 'running' AND started_at < ?`,
		deleteOlder: `DELETE FROM backup_drills WHERE id IN (SELECT id FROM backup_drills WHERE status != 'running' AND started_at < ? LIMIT ?)`,
	},
	RetentionProbeAttempts: {
		arg: argRFC3339Nano, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(probed_at), '') FROM probe_attempts`,
		countOlder:  `SELECT COUNT(*) FROM probe_attempts WHERE probed_at < ?`,
		deleteOlder: `DELETE FROM probe_attempts WHERE id IN (SELECT id FROM probe_attempts WHERE probed_at < ? LIMIT ?)`,
	},
	RetentionAppEvents: {
		arg: argEventTime, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(created_at), '') FROM app_events`,
		countOlder:  `SELECT COUNT(*) FROM app_events WHERE created_at < ?`,
		deleteOlder: `DELETE FROM app_events WHERE id IN (SELECT id FROM app_events WHERE created_at < ? LIMIT ?)`,
	},
	RetentionAppSchedule: {
		arg: argRFC3339Nano, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(created_at), '') FROM app_schedule_history`,
		countOlder:  `SELECT COUNT(*) FROM app_schedule_history WHERE created_at < ?`,
		deleteOlder: `DELETE FROM app_schedule_history WHERE id IN (SELECT id FROM app_schedule_history WHERE created_at < ? LIMIT ?)`,
	},
	RetentionCanaryOutcomes: {
		arg: argRFC3339, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(at), '') FROM canary_outcomes`,
		countOlder:  `SELECT COUNT(*) FROM canary_outcomes WHERE at < ?`,
		deleteOlder: `DELETE FROM canary_outcomes WHERE service_name IN (SELECT service_name FROM canary_outcomes WHERE at < ? LIMIT ?)`,
	},
	// A snapshot day is removed only when the whole day lies before the cutoff.
	RetentionUsageCostDaily: {
		arg: argDay, parse: parseDay,
		stats:       `SELECT COUNT(*), COALESCE(MIN(day), '') FROM usage_cost_daily`,
		countOlder:  `SELECT COUNT(*) FROM usage_cost_daily WHERE day < ?`,
		deleteOlder: `DELETE FROM usage_cost_daily WHERE rowid IN (SELECT rowid FROM usage_cost_daily WHERE day < ? LIMIT ?)`,
	},
	RetentionMCPOAuthRequests: {
		arg: argUnix, parse: parseUnix,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(expires_at) AS TEXT), '') FROM mcp_oauth_requests`,
		countOlder:  `SELECT COUNT(*) FROM mcp_oauth_requests WHERE expires_at < ?`,
		deleteOlder: `DELETE FROM mcp_oauth_requests WHERE rowid IN (SELECT rowid FROM mcp_oauth_requests WHERE expires_at < ? LIMIT ?)`,
	},
	// A used refresh row is kept for the window so a replay still reaches reuse detection.
	RetentionMCPOAuthRefresh: {
		arg: argUnix, parse: parseUnix,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(expires_at) AS TEXT), '') FROM mcp_oauth_refresh`,
		countOlder:  `SELECT COUNT(*) FROM mcp_oauth_refresh WHERE expires_at < ?1 OR used_at < ?1`,
		deleteOlder: `DELETE FROM mcp_oauth_refresh WHERE rowid IN (SELECT rowid FROM mcp_oauth_refresh WHERE expires_at < ?1 OR used_at < ?1 LIMIT ?2)`,
	},
	// Token rows go before their grant; a dead grant is revoked or past its hard expiry.
	RetentionMCPOAuthTokens: {
		arg: argUnix, parse: parseUnix,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(created_at) AS TEXT), '') FROM mcp_oauth_tokens`,
		countOlder:  `SELECT COUNT(*) FROM mcp_oauth_tokens WHERE grant_id IN (SELECT id FROM mcp_oauth_grants WHERE revoked_at < ?1 OR expires_at < ?1)`,
		deleteOlder: `DELETE FROM mcp_oauth_tokens WHERE rowid IN (SELECT rowid FROM mcp_oauth_tokens WHERE grant_id IN (SELECT id FROM mcp_oauth_grants WHERE revoked_at < ?1 OR expires_at < ?1) LIMIT ?2)`,
	},
	RetentionMCPOAuthGrants: {
		arg: argUnix, parse: parseUnix,
		stats:       `SELECT COUNT(*), COALESCE(CAST(MIN(created_at) AS TEXT), '') FROM mcp_oauth_grants`,
		countOlder:  `SELECT COUNT(*) FROM mcp_oauth_grants WHERE revoked_at < ?1 OR expires_at < ?1`,
		deleteOlder: `DELETE FROM mcp_oauth_grants WHERE rowid IN (SELECT rowid FROM mcp_oauth_grants WHERE revoked_at < ?1 OR expires_at < ?1 LIMIT ?2)`,
	},
	RetentionStatusIncidents: {
		arg: argRFC3339Nano, parse: parseDriverTime,
		stats:       `SELECT COUNT(*), COALESCE(MIN(updated_at), '') FROM status_incidents`,
		countOlder:  `SELECT COUNT(*) FROM status_incidents WHERE (status IN ('resolved', 'completed') OR (published = 0 AND internal_state = 'resolved')) AND updated_at < ?`,
		deleteOlder: `DELETE FROM status_incidents WHERE id IN (SELECT id FROM status_incidents WHERE (status IN ('resolved', 'completed') OR (published = 0 AND internal_state = 'resolved')) AND updated_at < ? LIMIT ?)`,
	},
}

// RetentionStats measures t: row count, oldest row time and on-disk size.
func (db *DB) RetentionStats(ctx context.Context, t RetentionTable) (CleanupTableStats, error) {
	spec, ok := retentionTables[t]
	if !ok {
		return CleanupTableStats{}, fmt.Errorf("store: %q is not a retention table", t)
	}
	var st CleanupTableStats
	var oldest string
	if err := db.QueryRowContext(ctx, spec.stats).Scan(&st.Rows, &oldest); err != nil {
		return st, fmt.Errorf("store: measure %s: %w", t, err)
	}
	if oldest != "" {
		st.Oldest, _ = spec.parse(oldest)
	}
	st.Bytes = db.tableBytes(ctx, string(t))
	return st, nil
}

// RetentionCountOlder counts the rows RetentionDeleteOlder would remove for the same cutoff.
func (db *DB) RetentionCountOlder(ctx context.Context, t RetentionTable, cutoff time.Time) (int64, error) {
	spec, ok := retentionTables[t]
	if !ok {
		return 0, fmt.Errorf("store: %q is not a retention table", t)
	}
	var n int64
	if err := db.QueryRowContext(ctx, spec.countOlder, spec.arg(cutoff)).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count old %s: %w", t, err)
	}
	return n, nil
}

// RetentionDeleteOlder removes finished rows of t strictly older than cutoff, batch rows per statement, and
// returns how many it removed. It stops between batches when ctx ends.
func (db *DB) RetentionDeleteOlder(ctx context.Context, t RetentionTable, cutoff time.Time, batch int) (int64, error) {
	spec, ok := retentionTables[t]
	if !ok {
		return 0, fmt.Errorf("store: %q is not a retention table", t)
	}
	if batch <= 0 {
		batch = retentionDefaultBatchSize
	}
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, fmt.Errorf("store: purge %s: %w", t, err)
		}
		res, err := db.ExecContext(ctx, spec.deleteOlder, spec.arg(cutoff), batch)
		if err != nil {
			return total, fmt.Errorf("store: purge %s: %w", t, err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n < int64(batch) {
			return total, nil
		}
	}
}
