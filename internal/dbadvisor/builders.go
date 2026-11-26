package dbadvisor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

const (
	// criticalFactor scales a configured threshold into the critical band.
	criticalFactor = 2
	// textMax caps statement and session text shown in a finding.
	textMax    = 300
	branchNote = " Test this on a database branch first; the advisor never runs it."
	docsBase   = "https://www.postgresql.org/docs/current/"
)

// UnusedIndexRow is one pg_stat_user_indexes row.
type UnusedIndexRow struct {
	Schema     string `json:"schema"`
	Table      string `json:"tbl"`
	Index      string `json:"index"`
	IdxScan    int64  `json:"idx_scan"`
	Bytes      int64  `json:"bytes"`
	Def        string `json:"def"`
	StatsReset string `json:"stats_reset"`
}

// FKRow is one foreign key whose columns have no leading index.
type FKRow struct {
	Schema     string   `json:"schema"`
	Table      string   `json:"tbl"`
	Constraint string   `json:"con_name"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"ref_table"`
	LiveTup    int64    `json:"n_live_tup"`
}

// TableStatRow is one pg_stat_user_tables row.
type TableStatRow struct {
	Schema         string `json:"schema"`
	Table          string `json:"tbl"`
	SeqScan        int64  `json:"seq_scan"`
	IdxScan        int64  `json:"idx_scan"`
	SeqTupRead     int64  `json:"seq_tup_read"`
	LiveTup        int64  `json:"n_live_tup"`
	DeadTup        int64  `json:"n_dead_tup"`
	LastAutovacuum string `json:"last_autovacuum"`
	LastVacuum     string `json:"last_vacuum"`
}

// CacheRow is the current database's pg_stat_database counters.
type CacheRow struct {
	BlksHit    int64  `json:"blks_hit"`
	BlksRead   int64  `json:"blks_read"`
	StatsReset string `json:"stats_reset"`
}

// StatementRow is one pg_stat_statements row.
type StatementRow struct {
	QueryID string  `json:"queryid"`
	Calls   int64   `json:"calls"`
	MeanMs  float64 `json:"mean_exec_time"`
	TotalMs float64 `json:"total_exec_time"`
	Rows    int64   `json:"rows"`
	Query   string  `json:"query"`
}

// StatementsState says whether pg_stat_statements can be read.
type StatementsState struct {
	Installed bool `json:"installed"`
	Preloaded bool `json:"preloaded"`
}

// SessionRow is one long or idle in transaction backend.
type SessionRow struct {
	PID         int64  `json:"pid"`
	User        string `json:"usename"`
	App         string `json:"application_name"`
	State       string `json:"state"`
	XactAgeSec  int64  `json:"xact_age_s"`
	StateAgeSec int64  `json:"state_age_s"`
	WaitType    string `json:"wait_event_type"`
	Wait        string `json:"wait_event"`
	Query       string `json:"query"`
}

// QuoteIdent quotes a Postgres identifier for suggestion text.
func QuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func qualified(schema, name string) string { return QuoteIdent(schema) + "." + QuoteIdent(name) }

func plainName(schema, name string) string { return schema + "." + name }

func quoteList(cols []string) string {
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = QuoteIdent(c)
	}
	return strings.Join(q, ", ")
}

var (
	// quotedRe matches single quoted and E'' literals; an unterminated one (a cut statement) runs to the end.
	quotedRe   = regexp.MustCompile(`(?s)'(?:[^'\\]|''|\\.)*(?:'|$)`)
	dollarOpen = regexp.MustCompile(`\$[A-Za-z_][A-Za-z_0-9]*\$|\$\$`)
	passwordRe = regexp.MustCompile(`(?is)\bpassword\b.*`)
	digitsRe   = regexp.MustCompile(`\b\d{4,}\b`)
)

// maskDollarQuoted hides dollar quoted bodies; an unterminated one runs to the end.
func maskDollarQuoted(s string) string {
	var b strings.Builder
	for {
		loc := dollarOpen.FindStringIndex(s)
		if loc == nil {
			b.WriteString(s)
			return b.String()
		}
		tag := s[loc[0]:loc[1]]
		b.WriteString(s[:loc[0]])
		b.WriteString("$?$")
		rest := s[loc[1]:]
		end := strings.Index(rest, tag)
		if end < 0 {
			return b.String()
		}
		s = rest[end+len(tag):]
	}
}

// maskLiterals hides everything that can carry a secret or personal data.
func maskLiterals(s string) string {
	s = passwordRe.ReplaceAllString(s, "password ?")
	s = maskDollarQuoted(s)
	s = quotedRe.ReplaceAllString(s, "'?'")
	return digitsRe.ReplaceAllString(s, "?")
}

// CleanText masks secrets, hides string literals, collapses whitespace, drops control characters and cuts to textMax runes.
func CleanText(s string) string {
	s = maskLiterals(secretmask.Mask(s))
	s = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)), " ")
	if utf8.RuneCountInString(s) > textMax {
		return string([]rune(s)[:textMax-3]) + "..."
	}
	return s
}

// BuildUnusedIndexes flags secondary indexes that were never scanned and are at least the configured size.
func BuildUnusedIndexes(rows []UnusedIndexRow, th Thresholds) []Finding {
	var out []Finding
	for _, r := range rows {
		if r.IdxScan != 0 || r.Bytes < th.UnusedIndexMinBytes {
			continue
		}
		out = append(out, Finding{
			ID: "unused_index:" + plainName(r.Schema, r.Index), Kind: KindUnusedIndex, Severity: SeverityWarn,
			Title:  "Index has never been used",
			Object: plainName(r.Schema, r.Index),
			Evidence: map[string]any{"table": plainName(r.Schema, r.Table), "idx_scan": r.IdxScan, "size_bytes": r.Bytes,
				"definition": CleanText(r.Def), "stats_since": orNever(r.StatsReset)},
			Suggestion: "DROP INDEX CONCURRENTLY " + qualified(r.Schema, r.Index) + ";" +
				" Counters only cover the time since the statistics were last reset, so check a full business cycle." + branchNote,
			Docs: docsBase + "monitoring-stats.html",
		})
	}
	return out
}

// BuildMissingFKIndexes flags foreign keys whose referencing columns have no index starting with them.
func BuildMissingFKIndexes(rows []FKRow, th Thresholds) []Finding {
	var out []Finding
	for _, r := range rows {
		if len(r.Columns) == 0 {
			continue
		}
		sev := SeverityInfo
		if r.LiveTup >= th.SeqScanMinRows {
			sev = SeverityWarn
		}
		out = append(out, Finding{
			ID: "missing_fk_index:" + plainName(r.Schema, r.Table) + ":" + r.Constraint, Kind: KindMissingFKIdx, Severity: sev,
			Title:    "Foreign key columns have no leading index",
			Object:   plainName(r.Schema, r.Table),
			Evidence: map[string]any{"constraint": r.Constraint, "columns": r.Columns, "references": r.RefTable, "table_live_rows": r.LiveTup},
			Suggestion: "CREATE INDEX CONCURRENTLY ON " + qualified(r.Schema, r.Table) + " (" + quoteList(r.Columns) + ");" +
				" Deletes and updates on the referenced table scan this table without it." + branchNote,
			Docs: docsBase + "ddl-constraints.html#DDL-CONSTRAINTS-FK",
		})
	}
	return out
}

// BuildSeqScans flags large tables read mostly by sequential scans.
func BuildSeqScans(rows []TableStatRow, th Thresholds) []Finding {
	var out []Finding
	for _, r := range rows {
		total := r.SeqScan + r.IdxScan
		// Sequential scans must have read at least a table's worth of rows, or one cheap scan would flag a table.
		if r.LiveTup < th.SeqScanMinRows || r.SeqScan <= 0 || total <= 0 || r.SeqTupRead < th.SeqScanMinRows {
			continue
		}
		pct := float64(r.SeqScan) * 100 / float64(total)
		if pct < float64(th.SeqScanRatioPercent) {
			continue
		}
		out = append(out, Finding{
			ID: "high_seq_scan:" + plainName(r.Schema, r.Table), Kind: KindSeqScan, Severity: SeverityWarn,
			Title:  "Table is mostly read by sequential scans",
			Object: plainName(r.Schema, r.Table),
			Evidence: map[string]any{"seq_scan": r.SeqScan, "idx_scan": r.IdxScan, "seq_scan_percent": round1(pct),
				"seq_tup_read": r.SeqTupRead, "live_rows": r.LiveTup},
			Suggestion: "Find the filters and joins that read " + qualified(r.Schema, r.Table) + " and add an index that matches them, for example " +
				"CREATE INDEX CONCURRENTLY ON " + qualified(r.Schema, r.Table) + " (<filtered column>);" + branchNote,
			Docs: docsBase + "indexes.html",
		})
	}
	return out
}

// BuildBloat flags tables whose dead tuples are a large share of their rows.
func BuildBloat(rows []TableStatRow, th Thresholds) []Finding {
	var out []Finding
	for _, r := range rows {
		if r.LiveTup+r.DeadTup < th.SeqScanMinRows || r.DeadTup <= 0 {
			continue
		}
		pct := float64(r.DeadTup) * 100 / float64(max(r.LiveTup, 1))
		if pct < float64(th.DeadTuplePercent) {
			continue
		}
		sev := SeverityWarn
		if pct >= float64(th.DeadTuplePercent*criticalFactor) {
			sev = SeverityCritical
		}
		out = append(out, Finding{
			ID: "dead_tuples:" + plainName(r.Schema, r.Table), Kind: KindBloat, Severity: sev,
			Title:  "Many dead tuples waiting for vacuum",
			Object: plainName(r.Schema, r.Table),
			Evidence: map[string]any{"dead_tuples": r.DeadTup, "live_tuples": r.LiveTup, "dead_percent": round1(pct),
				"last_autovacuum": r.LastAutovacuum, "last_vacuum": r.LastVacuum},
			Suggestion: "VACUUM (ANALYZE) " + qualified(r.Schema, r.Table) + ";" +
				" If this repeats, lower autovacuum_vacuum_scale_factor for the table with ALTER TABLE ... SET (...)." + branchNote,
			Docs: docsBase + "routine-vacuuming.html",
		})
	}
	return out
}

// BuildCacheHit flags a buffer cache hit ratio below the minimum.
func BuildCacheHit(row CacheRow, th Thresholds) []Finding {
	total := row.BlksHit + row.BlksRead
	if total <= 0 {
		return nil
	}
	pct := float64(row.BlksHit) * 100 / float64(total)
	if pct >= float64(th.CacheHitMinPercent) {
		return nil
	}
	sev := SeverityWarn
	if pct < float64(th.CacheHitMinPercent)/criticalFactor {
		sev = SeverityCritical
	}
	return []Finding{{
		ID: "low_cache_hit", Kind: KindCacheHit, Severity: sev,
		Title:  "Buffer cache hit ratio is low",
		Object: "database",
		Evidence: map[string]any{"hit_percent": round1(pct), "blks_hit": row.BlksHit, "blks_read": row.BlksRead,
			"min_percent": th.CacheHitMinPercent, "stats_since": orNever(row.StatsReset)},
		Suggestion: "Raise shared_buffers or give the database more memory, and check for large sequential scans. A freshly started or reset database reads cold, so recheck after it warms up." + branchNote,
		Docs:       docsBase + "runtime-config-resource.html",
	}}
}

// BuildStatements turns pg_stat_statements rows into findings, or explains how to enable the extension.
func BuildStatements(state StatementsState, rows []StatementRow, th Thresholds) []Finding {
	if !state.Installed || !state.Preloaded {
		return []Finding{{
			ID: "statements_not_enabled", Kind: KindStatementsOff, Severity: SeverityInfo,
			Title:    "pg_stat_statements not enabled",
			Evidence: map[string]any{"extension_created": state.Installed, "preloaded": state.Preloaded},
			Suggestion: "1. Add pg_stat_statements to shared_preload_libraries (this needs a database restart). " +
				"2. Restart the database. 3. Run CREATE EXTENSION IF NOT EXISTS pg_stat_statements; in this database. " +
				"The advisor then lists the slowest statements." + branchNote,
			Docs: docsBase + "pgstatstatements.html",
		}}
	}
	var out []Finding
	for _, r := range rows {
		if r.MeanMs < float64(th.SlowStatementMs) {
			continue
		}
		sev := SeverityInfo
		if r.MeanMs >= float64(th.SlowStatementMs*criticalFactor) {
			sev = SeverityWarn
		}
		out = append(out, Finding{
			ID: "slow_statement:" + r.QueryID, Kind: KindSlowStatement, Severity: sev,
			Title:  "Slow statement",
			Object: "queryid " + r.QueryID,
			Evidence: map[string]any{"mean_ms": round1(r.MeanMs), "calls": r.Calls, "total_ms": round1(r.TotalMs),
				"rows": r.Rows, "statement": CleanText(r.Query)},
			Suggestion: "Run EXPLAIN (ANALYZE, BUFFERS) on this statement with realistic parameters and look for sequential scans and row estimate misses." + branchNote,
			Docs:       docsBase + "using-explain.html",
		})
	}
	return out
}

// BuildSessions flags transactions open longer than the limit.
func BuildSessions(rows []SessionRow, th Thresholds) []Finding {
	limit := int64(th.IdleInTxMinutes) * 60
	var out []Finding
	for _, r := range rows {
		if r.XactAgeSec < limit {
			continue
		}
		sev := SeverityWarn
		if r.XactAgeSec >= limit*criticalFactor {
			sev = SeverityCritical
		}
		title := "Transaction has been open a long time"
		if strings.HasPrefix(r.State, "idle in transaction") {
			title = "Session is idle in transaction"
		}
		out = append(out, Finding{
			ID: fmt.Sprintf("long_session:%d", r.PID), Kind: KindLongSession, Severity: sev,
			Title:  title,
			Object: fmt.Sprintf("pid %d", r.PID),
			Evidence: map[string]any{"role": CleanText(r.User), "application": CleanText(r.App), "state": r.State,
				"transaction_age_seconds": r.XactAgeSec, "state_age_seconds": r.StateAgeSec,
				"wait_event_type": r.WaitType, "wait_event": r.Wait, "statement": CleanText(r.Query)},
			Suggestion: fmt.Sprintf("Find the client holding this session and fix its commit or rollback handling. If it must be stopped: SELECT pg_terminate_backend(%d);", r.PID) + branchNote,
			Docs:       docsBase + "runtime-config-client.html#GUC-IDLE-IN-TRANSACTION-SESSION-TIMEOUT",
		})
	}
	return out
}

// orNever words a null stats_reset, which means the counters were never reset.
func orNever(s string) string {
	if s == "" {
		return "never reset"
	}
	return s
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

var severityRank = map[string]int{SeverityInfo: 1, SeverityWarn: 2, SeverityCritical: 3}

// Order sorts by urgency and then object, and cuts to max findings.
func Order(fs []Finding, maxFindings int) ([]Finding, bool) {
	sort.SliceStable(fs, func(i, j int) bool {
		if a, b := severityRank[fs[i].Severity], severityRank[fs[j].Severity]; a != b {
			return a > b
		}
		if fs[i].Kind != fs[j].Kind {
			return fs[i].Kind < fs[j].Kind
		}
		return fs[i].Object < fs[j].Object
	})
	if maxFindings > 0 && len(fs) > maxFindings {
		return fs[:maxFindings], true
	}
	return fs, false
}
