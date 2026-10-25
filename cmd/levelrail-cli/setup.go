package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var setupStateLabel = map[string]string{
	"todo":           "todo",
	"in_progress":    "in progress",
	"done":           "done",
	"skipped":        "skipped",
	"not_applicable": "n/a",
}

var setupReasonText = map[string]string{
	"tls_terminated_upstream": "your proxy terminates TLS",
	"proxy_mode":              "served through your proxy",
	"unavailable":             "could not be read",
	"http_only":               "dashboard URL is plain http, switch to https",
	"no_alert_rule":           "channel added, create an alert rule",
	"certificates_pending":    "waiting for certificates",
	"acme_disabled":           "automatic certificates are off",
	"marked_done":             "marked done by an operator",
}

// runSetup implements "setup status|next|skip|undo|reset": the same Get
// started checklist the dashboard shows, from GET /api/v1/setup/guide.
func runSetup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, setupUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	sub := args[0]
	if sub == "admin" {
		return runSetupAdmin(prog, args[1:], stdout, stderr, lookupEnv)
	}
	switch sub {
	case "status", "next", "skip", "undo", "reset":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown setup command %q\n\n%s", prog, sub, setupUsage(prog))
		return exitUsage
	}

	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "setup "+sub, "print the checklist as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, setupUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	var (
		g   apiclient.SetupGuideResource
		err error
	)
	switch sub {
	case "skip", "undo":
		id, ok := requireOneArg(fs, stderr, prog, "setup "+sub, "step id")
		if !ok {
			return exitUsage
		}
		state := "skipped"
		if sub == "undo" {
			state = "todo"
		}
		g, err = client.SetSetupGuideStep(ctx, id, state)
	case "reset":
		g, err = client.ResetSetupGuide(ctx)
	default:
		g, err = client.GetSetupGuide(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("setup %s: %w", sub, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, g, func() {
		if sub == "next" {
			printSetupNext(stdout, g)
			return
		}
		printSetupStatus(stdout, g)
	})
}

func printSetupStatus(out io.Writer, g apiclient.SetupGuideResource) {
	_, _ = fmt.Fprintf(out, "Ready for production: %d%% (%d of %d steps done)\n\n", g.ReadyPercent, g.Done, g.Total)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATE\tSTEP\tID\tTIME\tNOTE")
	for _, s := range g.Steps {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", setupStateLabel[s.State], setupTitle(s), s.ID, setupMinutes(s), setupNote(s))
	}
	_ = tw.Flush()
	if g.Complete {
		_, _ = fmt.Fprintln(out, "\nEvery step is resolved.")
	}
}

func printSetupNext(out io.Writer, g apiclient.SetupGuideResource) {
	for _, s := range g.Steps {
		if s.ID != g.NextStep {
			continue
		}
		_, _ = fmt.Fprintf(out, "Next: %s (%s)\n", setupTitle(s), setupMinutes(s))
		if n := setupNote(s); n != "" {
			_, _ = fmt.Fprintf(out, "Note: %s\n", n)
		}
		_, _ = fmt.Fprintf(out, "Open: %s in the dashboard\n", s.Href)
		return
	}
	_, _ = fmt.Fprintf(out, "Nothing left to do. Ready for production: %d%%.\n", g.ReadyPercent)
}

func setupTitle(s apiclient.SetupGuideStep) string {
	if s.Optional {
		return s.Title + " (optional)"
	}
	return s.Title
}

func setupMinutes(s apiclient.SetupGuideStep) string {
	return fmt.Sprintf("~%d min", s.EstimateMinutes)
}

func setupNote(s apiclient.SetupGuideStep) string {
	switch {
	case len(s.BlockedBy) > 0:
		return "after " + s.BlockedBy[0]
	case s.Progress != nil:
		return fmt.Sprintf("%d of %d", s.Progress.Done, s.Progress.Total)
	}
	return setupReasonText[s.Reason]
}

func setupUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s setup status [flags]       the Get started checklist and how ready this server is for production
  %[1]s setup next [flags]         the one step to do next
  %[1]s setup skip <step> [flags]  mark a step as skipped (undo with "setup undo")
  %[1]s setup undo <step> [flags]  clear a skip or manual override
  %[1]s setup reset [flags]        clear every override and show the guide again
  %[1]s setup admin [--username U] [--api-url URL] [--profile NAME]  create the first admin on a fresh server (token from "setup-token" on the host)

Flags:
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the checklist as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
