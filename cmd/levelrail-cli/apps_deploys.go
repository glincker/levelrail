package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
	case "compare":
		return runAppsDeploysCompare(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps deploys subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsDeploysUsage(prog))
		return exitUsage
	}
}

func appsDeploysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys compare <name> --from ID [--to ID] [flags]   diff two deploy attempts, or one against the current live state

Run "%[1]s apps deploys <subcommand> -h" for a subcommand's own flags.
`, prog)
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
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps deploys compare", "print the comparison as JSON to stdout and nothing else", stderr)
	var from, to string
	fs.StringVar(&from, "from", "", "deploy attempt ID to compare from (required)")
	fs.StringVar(&to, "to", "", "deploy attempt ID to compare to (default: the app's current live state)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysCompareUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

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

	if jsonOut {
		if err := writeJSONValue(stdout, cmp); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printDeployCompareHuman(stdout, cmp)
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
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
