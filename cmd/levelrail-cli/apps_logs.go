package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// tailEntries applies --tail's client-side "last N entries" trim.
// Extracted from runAppsLogs as its own pure function for the same
// table-driven-test reason resolveTimeRange is: no client, no flags, no
// I/O, just a slice transform.
func tailEntries(entries []logEntryResource, tail int) []logEntryResource {
	if tail > 0 && len(entries) > tail {
		return entries[len(entries)-tail:]
	}
	return entries
}

// runAppsLogs implements "apps logs <name>": GET /api/v1/apps/{name}/logs
// (internal/api/logs.go's own handleQueryLogs), a real, historical
// full-text search over already-stored log entries, the same endpoint
// the web frontend's LogSearchPanel reads (web/src/queries/logs.ts).
//
// --follow switches to a live tail instead: GET
// /api/v1/apps/{name}/logs/stream (internal/api/live_logs.go's
// handleLiveLogStream), the same SSE connection the dashboard's live log
// viewer opens. It is mutually exclusive with every historical-search
// flag (--since/--from/--to/--q/--tail): the streaming endpoint takes no
// query params of its own (a fixed, short backfill plus everything from
// then on), so those flags would silently do nothing if allowed through.
//
// The server has no line-count query param on the historical search
// (its two filters are the from/to time window and q, a full-text
// phrase), so --tail is applied client-side, after the real query, by
// trimming to the last N entries of what the server returned.
func runAppsLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps logs", "print entries as a JSON array to stdout and nothing else", stderr)
	var since, from, to, query string
	var tail int
	var follow bool
	fs.StringVar(&since, "since", "", "how far back to search, e.g. \"1h\", \"30m\" (default: 1h, the server's own default window; mutually exclusive with --from and --follow)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the search window (overrides --since; mutually exclusive with --follow)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the search window (default: now; mutually exclusive with --follow)")
	fs.StringVar(&query, "q", "", "full-text search phrase; omitted means every log line in range (mutually exclusive with --follow)")
	fs.IntVar(&tail, "tail", 0, "only show the last N entries (applied client-side; mutually exclusive with --follow)")
	fs.BoolVar(&follow, "follow", false, "stream new log lines live, like \"docker logs -f\"; runs until Ctrl+C, ignores --since/--from/--to/--q/--tail/--query")
	fs.BoolVar(&follow, "f", false, "shorthand for --follow")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsLogsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps logs requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if follow {
		if since != "" || from != "" || to != "" || query != "" || tail != 0 || of.Query != "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--follow is a live tail and cannot be combined with --since/--from/--to/--q/--tail/--query"))
		}
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		return runAppsLogsFollow(client, name, stdout, stderr, of.Format)
	}

	if tail < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--tail must not be negative"))
	}

	fromTime, toTime, err := resolveTimeRange(timeRangeFlags{since: since, from: from, to: to}, time.Now())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	entries, err := client.QueryLogs(context.Background(), name, fromTime, toTime, query)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query logs for app %q: %w", name, err))
	}
	entries = tailEntries(entries, tail)

	if err := renderResult(stdout, of.Format, of.Query, entries, func() { printLogEntriesHuman(stdout, entries) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runAppsLogsFollow implements "apps logs <name> --follow": opens
// client.StreamLogs and prints each line as it arrives until ctx is
// canceled (Ctrl+C or SIGTERM, the same signal.NotifyContext shape
// cmd/levelrail's and cmd/levelrail-agent's own main() use) or the
// server closes the connection. format == outputJSON prints each entry
// as its own single-line JSON object (JSON Lines), since a live tail has
// no complete result set to marshal as one JSON array; every other
// format prints "STREAM LINE" the same way printLogEntriesHuman does,
// minus the timestamp column the streaming wire shape doesn't carry (see
// LogStreamEntry's own doc comment).
func runAppsLogsFollow(client *Client, name string, stdout, stderr io.Writer, format outputFormat) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := client.StreamLogs(ctx, name, func(e logStreamEntry) error {
		if format == outputJSON {
			return writeJSONLine(stdout, e)
		}
		_, err := fmt.Fprintf(stdout, "%s %s\n", e.Stream, e.Line)
		return err
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return reportError(stdout, stderr, format == outputJSON, fmt.Errorf("stream logs for app %q: %w", name, err))
	}
	return exitOK
}

func appsLogsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps logs <name> [flags]
  %[1]s apps logs <name> --follow [flags]

Searches an app's already-stored log entries (historical search). Add
--follow (or -f) to stream new lines live instead, the same SSE
connection the dashboard's live log viewer uses; it runs until Ctrl+C and
does not accept --since/--from/--to/--q/--tail/--query.

Output goes to stdout only (errors and usage go to stderr), so redirect
it to save a copy: %[1]s apps logs <name> --since 24h > app.log

Flags:
  --since string          how far back to search, e.g. "1h", "30m" (default: 1h)
  --from string            RFC3339 start of the search window (overrides --since)
  --to string                RFC3339 end of the search window (default: now)
  --q string                  full-text search phrase (default: every line in range)
  --tail int                  only show the last N entries (client-side)
  --follow, -f              stream new log lines live until Ctrl+C, instead of a historical search
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print entries as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
