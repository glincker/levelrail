package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runChannelsDelete implements "channels delete <id>": DELETE
// /api/v1/notification-channels/{id}. Never blocked by an app still
// attached (the server clears channel_id via ON DELETE SET NULL), so
// unlike a database or backup target this needs no confirmation flag.
func runChannelsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" channels delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print {\"deleted\": true} as JSON to stdout on success and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: channels delete requires exactly one channel id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	id := rest[0]

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	if err := client.DeleteNotificationChannel(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete notification channel %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "channel %q disconnected\n", id)
	return exitOK
}

func channelsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels delete <id> [flags]

Disconnects a notification channel. Any app currently attached to it
keeps its own deploy notify target row, with the channel reference
cleared, rather than being blocked or deleted.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
