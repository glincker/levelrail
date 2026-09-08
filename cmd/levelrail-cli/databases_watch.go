package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// runDatabasesWatch is runAppsWatch's database analogue: GET
// /api/v1/databases/{name}/watch. See that function's own doc comment
// for the signal handling and --json/--output/--query behavior, shared
// verbatim here.
func runDatabasesWatch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases watch", "print each condition change as one JSON line (JSON Lines) instead of text", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesWatchUsage(prog)) }

	client, name, jsonOut, _, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "databases watch", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := client.WatchDatabase(ctx, name, func(ev watchEventResource) {
		printWatchEvent(stdout, ev, jsonOut)
	})
	if err != nil && ctx.Err() == nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("watch database %q: %w", name, err))
	}
	return exitOK
}

func databasesWatchUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases watch <name> [flags]

Streams the database controller's reconcile conditions as they change,
one timestamped line per condition, until interrupted (Ctrl-C).

Flags:
  --json                     print each event as one JSON line (JSON Lines) instead of text
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
