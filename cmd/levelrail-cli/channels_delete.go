package main

import (
	"context"
	"fmt"
	"io"
)

// runChannelsDelete implements "channels delete <id>": DELETE
// /api/v1/notification-channels/{id}. Never blocked by an app still
// attached (the server clears channel_id via ON DELETE SET NULL), so
// unlike a database or backup target this needs no confirmation flag.
func runChannelsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "channels delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "channels delete", "channel id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteNotificationChannel(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete notification channel %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "channel %q disconnected\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
