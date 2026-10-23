package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/apiclient"
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
	case "wait":
		return runAppsDeploysWait(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runAppsDeploysShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "compare":
		return runAppsDeploysCompare(prog, args[1:], stdout, stderr, lookupEnv)
	case "release-notes":
		return runAppsDeploysReleaseNotes(prog, args[1:], stdout, stderr, lookupEnv)
	case "url":
		return runAppsDeploysURL(prog, args[1:], stdout, stderr, lookupEnv)
	case "logs":
		return runAppsDeploysLogs(prog, args[1:], stdout, stderr, lookupEnv)
	case "failed":
		return runAppsDeploysFailed(prog, args[1:], stdout, stderr, lookupEnv)
	case "timeline":
		return runAppsDeploysTimeline(prog, args[1:], stdout, stderr, lookupEnv)
	case "steps":
		return runAppsDeploysSteps(prog, args[1:], stdout, stderr, lookupEnv)
	case "probes":
		return runAppsDeploysProbes(prog, args[1:], stdout, stderr, lookupEnv)
	case "gate", "status":
		return runAppsDeploysGate(prog, args[1:], stdout, stderr, lookupEnv)
	case "cancel":
		return runAppsDeploysCancel(prog, args[1:], stdout, stderr, lookupEnv)
	case "rollback-to", "rollback", "promote":
		return runAppsDeploysRollbackTo(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps deploys subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsDeploysUsage(prog))
		return exitUsage
	}
}

func appsDeploysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys list <name> [flags]                          real, row-per-attempt deploy history, newest first
  %[1]s apps deploys wait <name> [deploy-id] [--timeout 10m]      block until a deploy is healthy or fails, exit 7 when it does not succeed
  %[1]s apps deploys show <name> [deploy-id] [flags]              one deploy attempt with its structured failure (newest by default)
  %[1]s apps deploys compare <name> --from ID [--to ID] [flags]   diff two deploy attempts, or one against the current live state
  %[1]s apps deploys release-notes <name> --from ID [--to ID]   markdown release notes from the commits between two deploys
  %[1]s apps deploys url <name> <deploy-id> [flags]               the deploy's own URL, to open a past deploy without rolling back
  %[1]s apps deploys logs <name> <deploy-id> [flags]              one deploy attempt's full build/log output
  %[1]s apps deploys failed [--since 24h] [flags]                 every app's latest failed deploy in the window, fleet-wide
  %[1]s apps deploys timeline [--since 24h] [flags]               fleet-wide deploys as bars on a shared time axis
  %[1]s apps deploys steps <name> <deploy-id> [flags]             one deploy attempt's pipeline steps, live until it ends
  %[1]s apps deploys probes <name> <deploy-id> [flags]            readiness-probe attempts a deploy's cutover made
  %[1]s apps deploys gate <name> [deploy-id] [--watch]            readiness gate, cutover, drain and post-deploy watch timeline
  %[1]s apps deploys cancel <name> <deploy-id> [flags]            cancel a queued or in-progress deploy before it cuts traffic
  %[1]s apps deploys rollback-to <name> <deploy-id> [flags]       redeploy a past succeeded deploy's exact image, pinned by digest
  %[1]s apps deploys promote <name> <deploy-id> [flags]           alias of rollback-to: make any past succeeded deploy the live one
  %[1]s apps deploys rollback <name> <deploy-id> [flags]          alias of rollback-to

Run "%[1]s apps deploys <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsDeploysList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys list", "print the attempt list as a JSON array to stdout and nothing else", stderr)
	var filter apiclient.DeployAttemptFilter
	fs.StringVar(&filter.Branch, "branch", "", "only deploys built from this git branch")
	fs.StringVar(&filter.Author, "author", "", "only deploys whose author contains this text")
	fs.StringVar(&filter.Commit, "commit", "", "only deploys whose commit SHA starts with this prefix")
	fs.StringVar(&filter.Status, "status", "", "only deploys with this status (succeeded, failed, running, ...)")
	fs.StringVar(&filter.Query, "search", "", "only deploys whose message, image, branch or author contains this text")
	fs.IntVar(&filter.Limit, "limit", 0, "show at most this many deploys (newest first)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysListUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps deploys list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	attempts, err := client.ListDeployAttemptsFiltered(context.Background(), name, filter)
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
  --branch string         only deploys built from this git branch
  --author string         only deploys whose author contains this text
  --commit string         only deploys whose commit SHA starts with this prefix
  --status string         only deploys with this status
  --search string         only deploys whose message, image, branch or author contains this text
  --limit int             show at most this many deploys
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the attempt list as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// Omitting --to compares --from against the app's current desired state.
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

// Always prints the not-tracked note, so terminal output matches the JSON and web views.
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

	printDeployCompareExtras(w, cmp)
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

// Without --follow this prints the saved log, so redirect the shell to save it.
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

// Returns on ctx cancel or server close, which also happens when the attempt already finished.
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
