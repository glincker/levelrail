package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runChannelsDeliveries implements "channels deliveries <id>": GET
// /api/v1/notification-channels/{id}/deliveries, this channel's send
// history (deploy outcomes, alert rules, and test-send clicks alike),
// newest first.
func runChannelsDeliveries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "channels deliveries", "print deliveries as a JSON array to stdout and nothing else", stderr)
	var limit int
	fs.IntVar(&limit, "limit", 0, "max rows to return (default: the server's own default)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsDeliveriesUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "channels deliveries", "channel id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	deliveries, err := client.ListNotificationDeliveries(context.Background(), id, limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list notification deliveries for channel %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, deliveries); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printNotificationDeliveriesTable(stdout, deliveries)
	return exitOK
}

func printNotificationDeliveriesTable(out io.Writer, deliveries []notificationDeliveryResource) {
	if len(deliveries) == 0 {
		_, _ = fmt.Fprintln(out, "no recorded deliveries")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TIME\tTRIGGER\tSTATUS\tERROR")
	for _, d := range deliveries {
		status := "ok"
		if !d.Success {
			status = "failed"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.CreatedAt, d.Trigger, status, d.Error)
	}
	_ = tw.Flush()
}

func channelsDeliveriesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels deliveries <id> [flags]

Lists a channel's recorded send history (deploy outcomes, alert rules,
and test sends), newest first.

Flags:
  --limit int             max rows to return (default: the server's own default)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print deliveries as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
