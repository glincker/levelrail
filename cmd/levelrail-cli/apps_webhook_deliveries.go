package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsWebhookDeliveries dispatches "apps webhook-deliveries <verb>
// [flags]" to one of list/replay, the CLI counterpart of
// internal/api/webhook_deliveries.go's own routes: real visibility into
// what a git provider actually sent to this app's webhook URL, and a
// manual replay of a stored delivery once whatever was wrong is fixed.
func runAppsWebhookDeliveries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsWebhookDeliveriesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsWebhookDeliveriesUsage(prog))
		return exitOK
	case "list":
		return runAppsWebhookDeliveriesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "replay":
		return runAppsWebhookDeliveriesReplay(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps webhook-deliveries subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsWebhookDeliveriesUsage(prog))
		return exitUsage
	}
}

func appsWebhookDeliveriesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps webhook-deliveries list <app-name> [flags]                 list recent inbound webhook requests
  %[1]s apps webhook-deliveries replay <app-name> <delivery-id> [flags]   re-run a stored delivery's payload

A delivery is recorded for every inbound git provider webhook request
this app's webhook URL receives, verified or not: bad signature, no git
source connected, or a downstream deploy failure are all visible here.
"replay" re-runs the exact same handling logic against the stored
payload, which can trigger a real build and deploy.

Run "%[1]s apps webhook-deliveries <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsWebhookDeliveriesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps webhook-deliveries list", "print deliveries as a JSON array to stdout and nothing else", stderr)
	var beforeFlag string
	var limitFlag int
	fs.IntVar(&limitFlag, "limit", 0, "max deliveries to return (default: server default)")
	fs.StringVar(&beforeFlag, "before", "", "only show deliveries received before this RFC3339 timestamp (page backward using the RECEIVED column of a prior run)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsWebhookDeliveriesListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps webhook-deliveries list", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	deliveries, err := client.ListWebhookDeliveries(context.Background(), appName, apiclient.ListWebhookDeliveriesOptions{
		Limit:  limitFlag,
		Before: beforeFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list webhook deliveries for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, deliveries, func() { printWebhookDeliveriesTable(stdout, deliveries) })
}

func printWebhookDeliveriesTable(out io.Writer, deliveries []webhookDeliveryResource) {
	if len(deliveries) == 0 {
		_, _ = fmt.Fprintln(out, "no webhook deliveries")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tPROVIDER\tEVENT\tSIGNATURE\tMATCHED\tSTATUS\tRECEIVED")
	for _, d := range deliveries {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%t\t%d\t%s\n",
			d.ID, d.Provider, d.EventType, d.SignatureValid, d.Matched, d.StatusCode, d.ReceivedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	_ = tw.Flush()
}

func appsWebhookDeliveriesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps webhook-deliveries list <app-name> [flags]

Lists an app's recent inbound git provider webhook requests, newest
first, whether or not they verified or matched a connected git source.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print deliveries as a JSON array to stdout, nothing else
  --limit int              max deliveries to return (default: server default)
  --before string          only show deliveries received before this RFC3339 timestamp
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsWebhookDeliveriesReplay(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps webhook-deliveries replay", "print the replay result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsWebhookDeliveriesReplayUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	positional, ok := requireArgs(fs, stderr, prog, "apps webhook-deliveries replay", "an app name and a delivery id", 2)
	if !ok {
		return exitUsage
	}
	appName, deliveryID := positional[0], positional[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.ReplayWebhookDelivery(context.Background(), appName, deliveryID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("replay webhook delivery %q for app %q: %w", deliveryID, appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "replayed delivery %q for app %q: status=%d message=%q\n", deliveryID, appName, result.Status, result.Message)
	})
}

func appsWebhookDeliveriesReplayUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps webhook-deliveries replay <app-name> <delivery-id> [flags]

Re-runs a stored delivery's exact payload through the same processing
a live webhook takes, which can trigger a real build and deploy.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the replay result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
