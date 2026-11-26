package dbadvisor

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
)

const systemFilter = `n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp%'`

// Every query is a fixed SELECT over the statistics views; only integer caps and thresholds are interpolated.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func unusedIndexesSQL(th Thresholds) string {
	return `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
SELECT s.schemaname AS schema, s.relname AS tbl, s.indexrelname AS index, s.idx_scan, pg_relation_size(s.indexrelid) AS bytes,
  pg_get_indexdef(s.indexrelid) AS def,
  (SELECT stats_reset::text FROM pg_stat_database WHERE datname = current_database()) AS stats_reset
FROM pg_stat_user_indexes s JOIN pg_index i ON i.indexrelid = s.indexrelid
WHERE s.idx_scan = 0 AND NOT i.indisunique AND NOT i.indisprimary AND NOT i.indisexclusion
  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = s.indexrelid)
  AND pg_relation_size(s.indexrelid) >= ` + itoa(th.UnusedIndexMinBytes) + `
ORDER BY pg_relation_size(s.indexrelid) DESC LIMIT ` + itoa(int64(th.MaxFindings)) + `) t`
}

// The leading columns of an index must be the same set as the key columns, in any order.
func missingFKSQL(th Thresholds) string {
	return `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
SELECT n.nspname AS schema, c.relname AS tbl, con.conname AS con_name,
  (SELECT json_agg(a.attname ORDER BY k.ord) FROM unnest(con.conkey) WITH ORDINALITY k(attnum, ord)
     JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.attnum) AS columns,
  con.confrelid::regclass::text AS ref_table,
  COALESCE((SELECT st.n_live_tup FROM pg_stat_user_tables st WHERE st.relid = con.conrelid), 0) AS n_live_tup
FROM pg_constraint con JOIN pg_class c ON c.oid = con.conrelid JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE con.contype = 'f' AND ` + systemFilter + `
  AND NOT EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = con.conrelid AND i.indisvalid
    AND (i.indkey::int2[])[0:cardinality(con.conkey) - 1] @> con.conkey
    AND (i.indkey::int2[])[0:cardinality(con.conkey) - 1] <@ con.conkey)
ORDER BY 6 DESC, n.nspname, c.relname LIMIT ` + itoa(int64(th.MaxFindings)) + `) t`
}

func tableStatsSQL(th Thresholds) string {
	return `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
SELECT schemaname AS schema, relname AS tbl, seq_scan, COALESCE(idx_scan, 0) AS idx_scan, seq_tup_read, n_live_tup, n_dead_tup,
  last_autovacuum::text AS last_autovacuum, last_vacuum::text AS last_vacuum
FROM pg_stat_user_tables
WHERE n_live_tup + n_dead_tup >= ` + itoa(th.SeqScanMinRows) + `
  AND ((seq_scan > 0 AND seq_scan * 100 >= ` + itoa(int64(th.SeqScanRatioPercent)) + ` * (seq_scan + COALESCE(idx_scan, 0)))
    OR (n_dead_tup > 0 AND n_dead_tup * 100 >= ` + itoa(int64(th.DeadTuplePercent)) + ` * GREATEST(n_live_tup, 1)))
ORDER BY seq_tup_read DESC, n_dead_tup DESC LIMIT ` + itoa(int64(th.MaxFindings)) + `) t`
}

const cacheSQL = `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
SELECT blks_hit, blks_read, stats_reset::text AS stats_reset FROM pg_stat_database WHERE datname = current_database()) t`

const statementsStateSQL = `SELECT json_build_object(
  'installed', EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements'),
  'preloaded', EXISTS (SELECT 1 FROM regexp_split_to_table(current_setting('shared_preload_libraries'), '\s*,\s*') AS l WHERE trim(both '"' from l) = 'pg_stat_statements'))`

func statementsSQL(th Thresholds) string {
	return `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
SELECT queryid::text AS queryid, calls, mean_exec_time, total_exec_time, rows, left(query, 2000) AS query
FROM pg_stat_statements
WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
  AND mean_exec_time >= ` + itoa(int64(th.SlowStatementMs)) + ` AND query NOT LIKE '%pg_stat_statements%'
ORDER BY total_exec_time DESC LIMIT ` + itoa(int64(th.MaxFindings)) + `) t`
}

func sessionsSQL(th Thresholds) string {
	return `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
SELECT pid, usename::text AS usename, application_name, state,
  EXTRACT(EPOCH FROM now() - xact_start)::bigint AS xact_age_s, EXTRACT(EPOCH FROM now() - state_change)::bigint AS state_age_s,
  wait_event_type, wait_event, left(query, 2000) AS query
FROM pg_stat_activity
WHERE datname = current_database() AND pid <> pg_backend_pid() AND backend_type = 'client backend'
  AND xact_start IS NOT NULL AND state IN ('active', 'idle in transaction', 'idle in transaction (aborted)')
  AND now() - xact_start >= make_interval(mins => ` + itoa(int64(th.IdleInTxMinutes)) + `)
ORDER BY xact_start LIMIT ` + itoa(int64(th.MaxFindings)) + `) t`
}

// Run executes every check and always returns a report: a failing check becomes a check_failed finding.
func Run(ctx context.Context, t dbviewer.Target, th Thresholds, now time.Time) (Report, error) {
	if t.Dialect != dbviewer.DialectPostgres {
		return Report{}, ErrUnsupported
	}
	parent := ctx
	if th.Timeout > 0 {
		t.Limits.Timeout = th.Timeout
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, th.Timeout*checkCount)
		defer cancel()
	}
	var all []Finding
	fail := func(check string, err error) {
		all = append(all, Finding{ID: "check_failed:" + check, Kind: KindCheckFailed, Severity: SeverityInfo,
			Title: "Check could not run: " + check, Object: check,
			Evidence: map[string]any{"reason": SafeReason(err)}})
	}

	var unused []UnusedIndexRow
	if err := queryRows(ctx, t, unusedIndexesSQL(th), &unused); err != nil {
		fail("unused indexes", err)
	} else {
		all = append(all, BuildUnusedIndexes(unused, th)...)
	}
	var fks []FKRow
	if err := queryRows(ctx, t, missingFKSQL(th), &fks); err != nil {
		fail("foreign key indexes", err)
	} else {
		all = append(all, BuildMissingFKIndexes(fks, th)...)
	}
	var tables []TableStatRow
	if err := queryRows(ctx, t, tableStatsSQL(th), &tables); err != nil {
		fail("table scans and dead tuples", err)
	} else {
		all = append(all, BuildSeqScans(tables, th)...)
		all = append(all, BuildBloat(tables, th)...)
	}
	var cache []CacheRow
	if err := queryRows(ctx, t, cacheSQL, &cache); err != nil {
		fail("cache hit ratio", err)
	} else if len(cache) > 0 {
		all = append(all, BuildCacheHit(cache[0], th)...)
	}
	all = append(all, statementFindings(ctx, t, th, fail)...)
	var sessions []SessionRow
	if err := queryRows(ctx, t, sessionsSQL(th), &sessions); err != nil {
		fail("long sessions", err)
	} else {
		all = append(all, BuildSessions(sessions, th)...)
	}

	// A caller that went away must not leave a report of failed checks in the cache.
	if err := parent.Err(); err != nil {
		return Report{}, err
	}
	out, truncated := Order(all, th.MaxFindings)
	if out == nil {
		out = []Finding{}
	}
	return Report{Engine: "postgres", CheckedAt: now.UTC(), Findings: out, Truncated: truncated}, nil
}

// checkCount scales the overall deadline: each check may use up to one timeout.
const checkCount = 8

func statementFindings(ctx context.Context, t dbviewer.Target, th Thresholds, fail func(string, error)) []Finding {
	var st StatementsState
	if err := queryOne(ctx, t, statementsStateSQL, &st); err != nil {
		fail("slow statements", err)
		return nil
	}
	if !st.Installed || !st.Preloaded {
		return BuildStatements(st, nil, th)
	}
	var rows []StatementRow
	if err := queryRows(ctx, t, statementsSQL(th), &rows); err != nil {
		fail("slow statements", err)
		return nil
	}
	return BuildStatements(st, rows, th)
}

func firstCell(res dbviewer.Result) (string, bool) {
	if len(res.Rows) == 0 || len(res.Rows[0]) == 0 || res.Rows[0][0] == nil {
		return "", false
	}
	return *res.Rows[0][0], true
}

func queryRows(ctx context.Context, t dbviewer.Target, sql string, dst any) error {
	return queryOne(ctx, t, sql, dst)
}

func queryOne(ctx context.Context, t dbviewer.Target, sql string, dst any) error {
	res, err := t.RunTrusted(ctx, sql)
	if err != nil {
		return err
	}
	cell, ok := firstCell(res)
	if !ok {
		return nil
	}
	return json.Unmarshal([]byte(cell), dst)
}

// SafeReason maps an error to fixed wording so database error text never reaches the report.
func SafeReason(err error) string {
	var qe *dbviewer.QueryError
	switch {
	case errors.Is(err, dbviewer.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.As(err, &qe):
		msg := strings.ToLower(qe.Message)
		switch {
		case strings.Contains(msg, "permission denied"), strings.Contains(msg, "insufficient privilege"):
			return "permission denied"
		case strings.Contains(msg, "does not exist"):
			return "not available on this server version"
		}
		return "the database rejected the query"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}
	return "query failed"
}
