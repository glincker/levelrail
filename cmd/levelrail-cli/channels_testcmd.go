package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runChannelsTest implements "channels test <id>": POST
// /api/v1/notification-channels/{id}/test, a real send against an
// already-connected channel's own stored kind/notify_url.
func runChannelsTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := channelsFlagSet(prog, "test", "print {\"sent\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsTestUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := channelsRequireID(fs, stderr, prog, "test")
	if !ok {
		return exitUsage
	}

	client := channelsClient(prog, apiURLFlag, tokenFlag, lookupEnv)

	if err := client.TestExistingNotificationChannel(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test notification channel %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"sent": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "test notification sent to channel %q\n", id)
	return exitOK
}

func channelsTestUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels test <id> [flags]

Sends a real test message to an already-connected channel.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {"sent": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
