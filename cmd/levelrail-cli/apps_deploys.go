package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// runAppsDeploys dispatches "apps deploys <verb> [flags]", the same
// one-verb-today, room-to-grow shape apps_webhook_deliveries.go already
// establishes for a nested app-scoped resource.
func runAppsDeploys(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsDeploysUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsDeploysUsage(prog))
		return exitOK
	case "list":
		return runAppsDeploysList(prog, args[1:], stdout, stderr, lookupEnv)
	case "compare":
		return runAppsDeploysCompare(prog, args[1:], stdout, stderr, lookupEnv)
	case "logs":
		return runAppsDeploysLogs(prog, args[1:], stdout, stderr, lookupEnv)
	case "failed":
		return runAppsDeploysFailed(prog, args[1:], stdout, stderr, lookupEnv)
	case "steps":
		return runAppsDeploysSteps(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps deploys subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsDeploysUsage(prog))
		return exitUsage
	}
}

func appsDeploysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys list <name> [flags]                          real, row-per-attempt deploy history, newest first
  %[1]s apps deploys compare <name> --from ID [--to ID] [flags]   diff two deploy attempts, or one against the current live state
  %[1]s apps deploys logs <name> <deploy-id> [flags]              one deploy attempt's full build/log output
  %[1]s apps deploys failed [--since 24h] [flags]                 every app's latest failed deploy in the window, fleet-wide
  %[1]s apps deploys steps <name> <deploy-id> [flags]             one deploy attempt's pipeline steps, live until it ends

Run "%[1]s apps deploys <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runAppsDeploysList implements "apps deploys list <name>": GET
// /api/v1/apps/{name}/deploy-attempts (internal/api/deploy_attempts.go's
// handleListDeployAttempts), the real row-per-trigger-call history the
// dashboard's own DeployAttemptsList.tsx already renders. Existed at the
// API layer with no CLI command reaching it at all until now: apps
// deploys compare's own doc comment used to point a caller here with
// "--json elsewhere" as the only way to actually get an ID, since there
// was no "elsewhere" in this CLI to point at.
func runAppsDeploysList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys list", "print the attempt list as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysListUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps deploys list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	attempts, err := client.ListDeployAttempts(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deploy attempts for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, attempts, func() { printDeployAttemptsHuman(stdout, attempts) })
}

func appsDeploysListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys list <name> [flags]

Lists name's real deploy history, newest first: one row per "apps
deploy"/"apps rollback"/webhook-triggered/compose-fan-out trigger, with
its id, image, source, status, and timing. Use an id from here with
"apps deploys compare --from ID" or "apps wait --attempt-id ID".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the attempt list as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsDeploysCompare implements "apps deploys compare <name> --from ID
// [--to ID]": GET /api/v1/apps/{name}/deploys/compare
// (internal/api/deploy_compare.go's handleCompareDeploys). --to is
// optional; omitting it compares --from against the app's current live
// desired state, the same "compare to current" shortcut the web
// frontend's comparison view offers. Deploy attempt IDs aren't looked up
// by this command (no client-side history-listing endpoint exists yet in
// this CLI, see apps_deploy.go's own doc comment on the same gap for
// rollback); find them via the dashboard's deploy history page or GET
// .../deploy-attempts with --json elsewhere.
func runAppsDeploysCompare(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys compare", "print the comparison as JSON to stdout and nothing else", stderr)
	var from, to string
	fs.StringVar(&from, "from", "", "deploy attempt ID to compare from (required)")
	fs.StringVar(&to, "to", "", "deploy attempt ID to compare to (default: the app's current live state)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysCompareUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys compare requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if from == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--from is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	cmp, err := client.CompareDeploys(context.Background(), name, from, to)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("compare deploys for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, cmp, func() { printDeployCompareHuman(stdout, cmp) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// printDeployCompareHuman renders cmp as a readable before/after summary:
// each side's identity line, the fields that actually changed, and the
// explicit not-tracked note so a terminal user gets the same honesty the
// JSON response and the web UI both carry.
func printDeployCompareHuman(w io.Writer, cmp deployCompareResource) {
	_, _ = fmt.Fprintf(w, "%s\n", cmp.ServiceName)
	_, _ = fmt.Fprintf(w, "  from: %s\n", deployCompareSideLabel(cmp.From))
	_, _ = fmt.Fprintf(w, "  to:   %s\n", deployCompareSideLabel(cmp.To))

	if len(cmp.Changes) == 0 {
		_, _ = fmt.Fprint(w, "\nno tracked fields differ\n")
	} else {
		_, _ = fmt.Fprint(w, "\nchanges:\n")
		for _, c := range cmp.Changes {
			_, _ = fmt.Fprintf(w, "  %-10s %s -> %s\n", c.Field, deployCompareValueLabel(c.From), deployCompareValueLabel(c.To))
		}
	}

	_, _ = fmt.Fprintf(w, "\nnot tracked per deploy attempt: %v\n%s\n", cmp.UnsnapshottedFields, cmp.Note)
}

func deployCompareSideLabel(s deployCompareSide) string {
	if s.IsCurrent {
		return fmt.Sprintf("current live state (image %s)", s.Image)
	}
	return fmt.Sprintf("%s (image %s, status %s)", s.DeployID, s.Image, s.Status)
}

func deployCompareValueLabel(v string) string {
	if v == "" {
		return "(none)"
	}
	return v
}

func appsDeploysCompareUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys compare <name> --from ID [--to ID] [flags]

Diffs two deploy attempts (--from, --to), or --from against the app's
current live desired state when --to is omitted. Only the fields
store.DeployAttempt actually records (image, commit, trigger source,
outcome) are compared; environment variables, ports, domains, resource
limits, and other service configuration are never snapshotted per
attempt and so cannot be diffed across past deploys, only reported as
not tracked. Use "%[1]s apps rollback <name> --image IMAGE" to act on
what you see here.

Flags:
  --from string           deploy attempt ID to compare from (required)
  --to string              deploy attempt ID to compare to (default: current live state)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the comparison as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsDeploysLogs implements "apps deploys logs <name> <deploy-id>":
// GET /api/v1/apps/{name}/deploys/{deployId}/logs/download
// (internal/api/deploy_log_download.go's handleDownloadDeployLog),
// writing the attempt's raw build/log output straight to stdout. No
// --json/--output/--query here: unlike this command group's other two
// subcommands, there is no structured alternative to a deploy's own log
// text, so shell redirection ("> file.txt") is the intended way to
// save it, the same convention "apps logs" already leaves to the shell
// rather than adding its own --download-to flag.
//
// --follow switches to a live tail instead: GET
// /api/v1/apps/{name}/deploys/{deployId}/logs
// (internal/api/deploy_attempts.go's handleDeployLogStream), the same SSE
// connection the dashboard's deploy log page opens, the exact live-tail
// shape "apps logs --follow" already establishes for the app-log
// equivalent.
func runAppsDeploysLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys logs", "with --follow, print one JSON object per line instead of raw log text; not applicable otherwise", stderr)
	var follow bool
	fs.BoolVar(&follow, "follow", false, "stream the deploy's log live, like \"docker logs -f\"; runs until Ctrl+C or the attempt finishes")
	fs.BoolVar(&follow, "f", false, "shorthand for --follow")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysLogsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps deploys logs", "an app name and a deploy attempt id", 2)
	if !ok {
		return exitUsage
	}
	name, deployID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if follow {
		return runAppsDeploysLogsFollow(client, name, deployID, stdout, stderr, jsonOut)
	}

	data, err := client.DownloadDeployLog(context.Background(), name, deployID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("download deploy log for %s/%s: %w", name, deployID, err))
	}
	_, _ = stdout.Write(data)
	return exitOK
}

// runAppsDeploysLogsFollow implements "apps deploys logs <name>
// <deploy-id> --follow", mirroring runAppsLogsFollow (apps_logs.go): opens
// client.StreamDeployLog and prints each line as it arrives until ctx is
// canceled (Ctrl+C or SIGTERM) or the server closes the connection
// (the attempt finished and this was a live tail, or it was already
// finished and this was just a one-shot replay of the persisted log; see
// handleDeployLogStream's own doc comment for which).
func runAppsDeploysLogsFollow(client *Client, name, deployID string, stdout, stderr io.Writer, jsonOut bool) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := client.StreamDeployLog(ctx, name, deployID, func(e logStreamEntry) error {
		if jsonOut {
			return writeJSONLine(stdout, e)
		}
		_, err := fmt.Fprintf(stdout, "%s %s\n", e.Stream, e.Line)
		return err
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("stream deploy log for %s/%s: %w", name, deployID, err))
	}
	return exitOK
}

func appsDeploysLogsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys logs <name> <deploy-id> [flags]
  %[1]s apps deploys logs <name> <deploy-id> --follow [flags]

Prints deploy-id's full build/log output to stdout: live-buffered lines
so far if the attempt is still running, the full persisted log
otherwise. Find a deploy-id with "apps deploys list <name>". Redirect
to a file to save it ("%[1]s apps deploys logs <name> <deploy-id> >
build.log").

Add --follow (or -f) to stream the log live instead, the same SSE
connection the dashboard's deploy log page uses; it runs until Ctrl+C
or the attempt finishes.

Flags:
  --follow, -f              stream the deploy's log live until Ctrl+C or the attempt finishes
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    with --follow, print one JSON object per line instead of "stream line" text
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
