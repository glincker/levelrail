package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

// defaultLogsWindow mirrors internal/api's own defaultQueryWindow
// (internal/api/metrics.go), the lookback the server applies when a
// caller omits "from". Duplicated as a plain constant here rather than
// imported for the same reason client.go's own wire-shape types are
// redeclared rather than imported: this binary depends only on the
// documented wire contract, never on the control plane's internal Go
// values. If the server's own default ever changes, this local default
// only matters for what "apps logs" shows without --since/--from; every
// request still sends an explicit "from", so it can't silently disagree
// with the server about what "no time window given" means.
const defaultLogsWindow = time.Hour

// logsWindowFlags is resolveLogWindow's raw, unvalidated input: exactly
// what flag.FlagSet parsed for --since/--from/--to, no time math done
// yet. Kept separate so resolveLogWindow is a pure function over plain
// data, testable without a flag.FlagSet or the real clock in the loop.
type logsWindowFlags struct {
	since string
	from  string
	to    string
}

// resolveLogWindow turns f into the concrete [from, to] window
// client.QueryLogs sends, applying the --since > --from > default-window
// precedence the flags' own usage text documents. now stands in for
// time.Now() so this is deterministic and testable.
func resolveLogWindow(f logsWindowFlags, now time.Time) (from, to time.Time, err error) {
	if f.since != "" && f.from != "" {
		return time.Time{}, time.Time{}, newValidationError("--since and --from are mutually exclusive")
	}

	to = now
	if f.to != "" {
		parsed, parseErr := time.Parse(time.RFC3339, f.to)
		if parseErr != nil {
			return time.Time{}, time.Time{}, newValidationError("--to must be RFC3339: %v", parseErr)
		}
		to = parsed
	}

	from = to.Add(-defaultLogsWindow)
	switch {
	case f.from != "":
		parsed, parseErr := time.Parse(time.RFC3339, f.from)
		if parseErr != nil {
			return time.Time{}, time.Time{}, newValidationError("--from must be RFC3339: %v", parseErr)
		}
		from = parsed
	case f.since != "":
		d, parseErr := time.ParseDuration(f.since)
		if parseErr != nil {
			return time.Time{}, time.Time{}, newValidationError("--since must be a valid duration (e.g. \"1h\"): %v", parseErr)
		}
		from = to.Add(-d)
	}
	return from, to, nil
}

// tailEntries applies --tail's client-side "last N entries" trim.
// Extracted from runAppsLogs as its own pure function for the same
// table-driven-test reason resolveLogWindow is: no client, no flags, no
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
// This is search, not a live tail. There is no streaming log route wired
// up server-side today: web/src/hooks/useDeployLogStream.ts's own header
// comment documents a GET .../deploys/{deployId}/logs SSE contract as
// "assumed" and un-confirmed against a real handler, and
// internal/api/router.go registers no such route, only GET
// .../apps/{name}/logs above. So this command deliberately has no
// --follow flag; adding one against a route that doesn't exist would be
// exactly the kind of client built against an unconfirmed endpoint this
// CLI's own design brief rules out.
//
// The server has no line-count query param (its two filters are the
// from/to time window and q, a full-text phrase), so --tail is applied
// client-side, after the real query, by trimming to the last N entries
// of what the server returned.
func runAppsLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps logs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, since, from, to, query string
	var tail int
	var jsonOut bool
	fs.StringVar(&since, "since", "", "how far back to search, e.g. \"1h\", \"30m\" (default: 1h, the server's own default window; mutually exclusive with --from)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the search window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the search window (default: now)")
	fs.StringVar(&query, "q", "", "full-text search phrase; omitted means every log line in range")
	fs.IntVar(&tail, "tail", 0, "only show the last N entries (applied client-side; the server has no line-count param)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	var profileFlag string
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print entries as a JSON array to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsLogsUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps logs requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if tail < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--tail must not be negative"))
	}

	fromTime, toTime, err := resolveLogWindow(logsWindowFlags{since: since, from: from, to: to}, time.Now())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))

	entries, err := client.QueryLogs(context.Background(), name, fromTime, toTime, query)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query logs for app %q: %w", name, err))
	}
	entries = tailEntries(entries, tail)

	if jsonOut {
		if err := writeJSONValue(stdout, entries); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printLogEntriesHuman(stdout, entries)
	return exitOK
}

func appsLogsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps logs <name> [flags]

Searches an app's already-stored log entries (historical search, not a
live tail; there is no streaming log route today).

Output goes to stdout only (errors and usage go to stderr), so redirect
it to save a copy: %[1]s apps logs <name> --since 24h > app.log

Flags:
  --since string          how far back to search, e.g. "1h", "30m" (default: 1h)
  --from string            RFC3339 start of the search window (overrides --since)
  --to string                RFC3339 end of the search window (default: now)
  --q string                  full-text search phrase (default: every line in range)
  --tail int                  only show the last N entries (client-side)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print entries as a JSON array to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
