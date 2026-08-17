package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runChannelsList implements "channels list": GET
// /api/v1/notification-channels.
func runChannelsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" channels list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print channels as a JSON array to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	channels, err := client.ListNotificationChannels(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list notification channels: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, channels); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printNotificationChannelsTable(stdout, channels)
	return exitOK
}

func printNotificationChannelsTable(out io.Writer, channels []notificationChannelResource) {
	if len(channels) == 0 {
		_, _ = fmt.Fprintln(out, "no notification channels")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tKIND\tENABLED\tCREATED")
	for _, c := range channels {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n", c.ID, c.Name, c.Kind, c.Enabled, c.CreatedAt)
	}
	_ = tw.Flush()
}

func channelsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels list [flags]

Lists every connected notification channel.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print channels as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
