package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// runAppsWatch implements "apps watch <name>": GET
// /api/v1/apps/{name}/watch (internal/api/app_watch.go's own
// handleAppWatch), an SSE stream of the application controller's stored
// reconcile conditions, live rather than the one-shot read "apps status"
// makes. Runs until the user hits Ctrl-C (SIGINT) or the process is
// asked to stop (SIGTERM), matching cmd/levelrail-mcp/main.go's own
// signal.NotifyContext shutdown pattern, the only other place in this
// project that turns an OS signal into context cancellation.
//
// --output and --query don't apply here: there is no single result to
// format as a table or filter with a JMESPath expression, only an
// unbounded sequence of events. --json still means something: printing
// each event as one compact JSON line (JSON Lines) instead of the
// default timestamped-per-condition text.
func runAppsWatch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps watch", "print each condition change as one JSON line (JSON Lines) instead of text", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsWatchUsage(prog)) }

	client, name, jsonOut, _, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps watch", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := client.WatchApp(ctx, name, func(ev watchEventResource) {
		printWatchEvent(stdout, ev, jsonOut)
	})
	if err != nil && ctx.Err() == nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("watch app %q: %w", name, err))
	}
	return exitOK
}

// printWatchEvent prints one watch event: as a JSON Lines object when
// jsonOut is set, otherwise as one timestamped "TYPE STATUS REASON" line
// per condition in ev, e.g. "2026-09-08T12:00:00Z  Ready  True  Deployed".
func printWatchEvent(out io.Writer, ev watchEventResource, jsonOut bool) {
	if jsonOut {
		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintln(out, string(data))
		return
	}
	ts := ev.ObservedAt.UTC().Format(time.RFC3339)
	for _, c := range ev.Conditions {
		_, _ = fmt.Fprintf(out, "%s  %s  %s  %s\n", ts, c.Type, c.Status, c.Reason)
	}
}

func appsWatchUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps watch <name> [flags]

Streams the application controller's reconcile conditions as they
change (e.g. Ready flipping False to True after a deploy, or a new
crashloop condition appearing), one timestamped line per condition,
until interrupted (Ctrl-C).

Flags:
  --json                     print each event as one JSON line (JSON Lines) instead of text
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
