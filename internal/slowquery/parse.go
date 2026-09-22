// Package slowquery parses already-stored container log lines (see
// internal/api/database_slow_queries.go) into structured slow-query
// entries for Postgres and MySQL. Redis has no log-based slow query
// record (SLOWLOG is a live-server command, not log output) and is
// deliberately not handled here.
package slowquery

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogLine is one stored container log line: its own capture timestamp
// (not the engine's in-message timestamp text) and raw message.
type LogLine struct {
	Timestamp time.Time
	Message   string
}

// Entry is one parsed slow query.
type Entry struct {
	Timestamp time.Time
	// DurationMs is the statement's own reported execution time in
	// milliseconds (Postgres' "duration:", MySQL's "Query_time"
	// converted from seconds).
	DurationMs float64
	Query      string
	// RowsExamined is 0 when the engine's slow query log doesn't report
	// it (Postgres never does; MySQL always does for a real slow-log
	// entry).
	RowsExamined int64
}

// postgresLogPattern matches a log_min_duration_statement log line, e.g.
// `... LOG:  duration: 1234.567 ms  statement: SELECT 1`. A query
// wrapped across multiple lines is not stitched back together.
var postgresLogPattern = regexp.MustCompile(`duration:\s*([0-9.]+)\s*ms\s+(?:statement|execute\s+\S+):\s*(.*)$`)

// ParsePostgres extracts every slow-statement entry from lines, in the
// order given. Non-matching lines (ordinary Postgres log output that
// isn't a duration line) are skipped, not treated as an error: the raw
// log stream mixes slow-query lines with every other message Postgres
// writes to stderr.
func ParsePostgres(lines []LogLine) []Entry {
	var out []Entry
	for _, l := range lines {
		m := postgresLogPattern.FindStringSubmatch(l.Message)
		if m == nil {
			continue
		}
		durationMs, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		query := strings.TrimSpace(m[2])
		if query == "" {
			continue
		}
		out = append(out, Entry{
			Timestamp:  l.Timestamp,
			DurationMs: durationMs,
			Query:      query,
		})
	}
	return out
}

// mysqlQueryTimePattern matches the MySQL/MariaDB slow query log's
// header line for one entry, produced when slow_query_log/long_query_time
// are set (see internal/reconcile/database/controller.go's
// mysqlCommand):
//
//	# Query_time: 1.234567  Lock_time: 0.000123 Rows_sent: 1  Rows_examined: 1000000
var mysqlQueryTimePattern = regexp.MustCompile(`^#\s*Query_time:\s*([0-9.]+)\s+Lock_time:\s*[0-9.]+\s+Rows_sent:\s*[0-9]+\s+Rows_examined:\s*([0-9]+)`)

// mysqlUseDBPattern matches the "use <db>;" context line MySQL's slow
// log emits per entry whenever the connection's default database was
// just set (confirmed against a real container: a client that connects
// with a default schema logs this line before "SET timestamp=..."), a
// database-context marker carrying no query text of its own.
var mysqlUseDBPattern = regexp.MustCompile(`(?i)^use\s+\S+;$`)

// mysqlSetTimestampPattern matches the "SET timestamp=<unix-seconds>;"
// line MySQL's slow log always emits per entry: the query actually ran
// at this time, which ParseMySQL uses as the entry's Timestamp in place
// of the caller-supplied LogLine.Timestamp (accurate when reading the
// log file directly via exec, where LogLine.Timestamp carries no real
// per-line time at all; still more accurate than a Docker log-capture
// timestamp when both are available).
var mysqlSetTimestampPattern = regexp.MustCompile(`^SET timestamp=([0-9]+);$`)

// ParseMySQL extracts every slow-query entry from lines, in the order
// given, reconstructing each entry's multi-line block: a "# Query_time:"
// header, an optional "# User@Host:"/"SET timestamp=...;" line, then one
// or more lines of SQL text terminated by the next header line (another
// "#"-prefixed line) or the end of input. Everything before the first
// header line, and any "#"-prefixed line that isn't a header this
// package recognizes (e.g. "# Time:", "# User@Host:"), is skipped.
func ParseMySQL(lines []LogLine) []Entry {
	var out []Entry
	var current *Entry
	var queryParts []string

	flush := func() {
		if current == nil {
			return
		}
		query := strings.TrimSpace(strings.Join(queryParts, " "))
		query = strings.TrimSuffix(query, ";")
		query = strings.TrimSpace(query)
		if query != "" {
			current.Query = query
			out = append(out, *current)
		}
		current = nil
		queryParts = nil
	}

	for _, l := range lines {
		if m := mysqlQueryTimePattern.FindStringSubmatch(l.Message); m != nil {
			flush()
			durationSec, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			rowsExamined, _ := strconv.ParseInt(m[2], 10, 64)
			current = &Entry{
				Timestamp:    l.Timestamp,
				DurationMs:   durationSec * 1000,
				RowsExamined: rowsExamined,
			}
			continue
		}
		if current == nil {
			continue
		}
		trimmed := strings.TrimSpace(l.Message)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			// A different header line (e.g. "# Time:", "# User@Host:")
			// belonging to this same entry: carries no query text.
			continue
		}
		if m := mysqlSetTimestampPattern.FindStringSubmatch(trimmed); m != nil {
			if epoch, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				current.Timestamp = time.Unix(epoch, 0).UTC()
			}
			continue
		}
		if mysqlUseDBPattern.MatchString(trimmed) {
			continue
		}
		queryParts = append(queryParts, trimmed)
	}
	flush()

	return out
}
