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

	"golang.org/x/term"
)

// stdoutIsTerminal is a variable so tests can stand in for a terminal.
var stdoutIsTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd())) //nolint:gosec // a terminal file descriptor always fits an int
}

// tailEntries applies --tail as a client-side trim, since the server has no line-count parameter.
func tailEntries(entries []logEntryResource, tail int) []logEntryResource {
	if tail > 0 && len(entries) > tail {
		return entries[len(entries)-tail:]
	}
	return entries
}

// runAppsLogs implements "apps logs <name>" over the historical search endpoint.
// --follow switches to the SSE live tail, which takes no query params, so the search flags are rejected with it.
func runAppsLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps logs", "print entries as a JSON array to stdout and nothing else", stderr)
	var since, from, to, query, service string
	var tail int
	var follow, noFollow bool
	fs.StringVar(&service, "service", "", "only this service of a multi-service app (reads <app>-<service>)")
	fs.BoolVar(&noFollow, "no-follow", false, "print stored entries and exit even on a terminal (a terminal otherwise follows live)")
	fs.StringVar(&since, "since", "", "how far back to search, e.g. \"1h\", \"30m\" (default: 1h, the server's own default window; mutually exclusive with --from and --follow)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the search window (overrides --since; mutually exclusive with --follow)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the search window (default: now; mutually exclusive with --follow)")
	fs.StringVar(&query, "q", "", "search expression: words, \"exact phrase\", prefix*, OR, NOT, -word, parentheses; omitted means every log line in range (mutually exclusive with --follow)")
	fs.IntVar(&tail, "tail", 0, "only show the last N entries (applied client-side; mutually exclusive with --follow)")
	fs.IntVar(&tail, "n", 0, "shorthand for --tail")
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
	name := serviceAppName(rest[0], service)

	// A terminal with no search flags wants the live tail; pipes and --json keep the stored-entries output.
	searching := since != "" || from != "" || to != "" || query != "" || tail != 0 || of.Query != ""
	if !follow && !noFollow && !searching && !jsonOut && of.Format != outputJSON && stdoutIsTerminal(stdout) {
		follow = true
	}
	if follow && noFollow {
		return reportError(stdout, stderr, jsonOut, newValidationError("--follow and --no-follow contradict each other"))
	}

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
	if len(entries) == 0 && !jsonOut {
		hintBuildingDeploy(client, prog, name, stderr)
	}

	if err := renderResult(stdout, of.Format, of.Query, entries, func() { printLogEntriesHuman(stdout, entries) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runAppsLogsFollow streams lines until ctx is canceled or the server closes the stream. JSON output is JSON Lines.
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

On a terminal with no search flags the command follows live by default;
pass --no-follow for stored entries. Piped output and --json never follow.
Use --service S for one service of a multi-service app.

Output goes to stdout only (errors and usage go to stderr), so redirect
it to save a copy: %[1]s apps logs <name> --since 24h > app.log

Flags:
  --since string          how far back to search, e.g. "1h", "30m" (default: 1h)
  --from string            RFC3339 start of the search window (overrides --since)
  --to string                RFC3339 end of the search window (default: now)
  --q string                  full-text search phrase (default: every line in range)
  --tail, -n int         only show the last N entries (client-side)
  --service string       only this service of a multi-service app
  --no-follow            stored entries even on a terminal
  --follow, -f              stream new log lines live until Ctrl+C, instead of a historical search
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print entries as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

// hintBuildingDeploy points at the build log when an app has no runtime logs because its first deploy is still building.
func hintBuildingDeploy(client *Client, prog, name string, stderr io.Writer) {
	attempts, err := client.ListDeployAttempts(context.Background(), name)
	if err != nil {
		return
	}
	for _, a := range attempts {
		if a.Status == "building" || a.Status == "queued" {
			_, _ = fmt.Fprintf(stderr, "no container logs yet, deploy %s is %s: %s\n", a.ID, a.Status, followHint(prog, name, a.ID))
			return
		}
	}
}
