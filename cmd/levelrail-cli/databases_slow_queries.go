package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runDatabasesSlowQueries implements "databases slow-queries <name>": GET
// /api/v1/databases/{name}/slow-queries
// (internal/api/database_slow_queries.go's
// handleQueryDatabaseSlowQueries), the database-kind slow-query-log
// counterpart to "databases logs" (databases_logs.go). Only Postgres and
// MySQL databases support this; the server returns 400 for every other
// engine, surfaced here the same way any other API error is.
func runDatabasesSlowQueries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases slow-queries", "print entries as a JSON array to stdout and nothing else", stderr)
	var since, from, to string
	var limit, offset int
	fs.StringVar(&since, "since", "", "how far back to search, e.g. \"1h\", \"30m\" (default: 1h, the server's own default window)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the search window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the search window (default: now)")
	fs.IntVar(&limit, "limit", 0, "max entries to return, sorted by duration descending (default: server default, 100)")
	fs.IntVar(&offset, "offset", 0, "skip this many entries before returning --limit of them")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesSlowQueriesUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: databases slow-queries requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if offset < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--offset must not be negative"))
	}

	fromTime, toTime, err := resolveTimeRange(timeRangeFlags{since: since, from: from, to: to}, time.Now())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	entries, total, err := client.QueryDatabaseSlowQueries(context.Background(), name, fromTime, toTime, limit, offset)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query slow queries for database %q: %w", name, err))
	}

	result := struct {
		Entries []slowQueryEntryResource `json:"entries"`
		Total   int                      `json:"total"`
	}{Entries: entries, Total: total}

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printSlowQueryEntriesHuman(stdout, entries, total) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func databasesSlowQueriesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases slow-queries <name> [flags]

Lists slow query log entries for a Postgres or MySQL database (parsed
from the database's own container logs), sorted by duration descending.
Every other engine returns an error: Redis has no comparable log-based
slow query record.

Output goes to stdout only (errors and usage go to stderr).

Flags:
  --since string          how far back to search, e.g. "1h", "30m" (default: 1h)
  --from string            RFC3339 start of the search window (overrides --since)
  --to string                RFC3339 end of the search window (default: now)
  --limit int                max entries to return, sorted by duration descending (default: 100)
  --offset int               skip this many entries before returning --limit of them
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print entries as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
