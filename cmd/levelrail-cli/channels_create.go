package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runChannelsCreate implements "channels create --name NAME --kind
// KIND": POST /api/v1/notification-channels. Every kind takes
// --notify-url directly (a webhook URL, a Telegram sendMessage URL with
// chat_id, an email address, ...); --kind pushover additionally accepts
// --pushover-user-key/--pushover-api-token as a convenience so an
// operator supplies Pushover's own two credentials separately instead
// of hand-building the query-string notify_url the API expects
// (mirroring how the web dashboard's connect dialog does the same
// packing client-side).
func runChannelsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "channels create", "print the created channel as JSON to stdout and nothing else", stderr)
	var name, kind, notifyURL, pushoverUserKey, pushoverAPIToken string
	var disabled bool
	fs.StringVar(&name, "name", "", "display name for the channel (required)")
	fs.StringVar(&kind, "kind", "", "channel kind: generic, slack, discord, telegram, email, pushover (required)")
	fs.StringVar(&notifyURL, "notify-url", "", "destination (webhook URL, Telegram sendMessage URL, or email address); required unless --kind pushover with both --pushover-user-key and --pushover-api-token set")
	fs.StringVar(&pushoverUserKey, "pushover-user-key", "", "Pushover User Key (--kind pushover only, an alternative to --notify-url)")
	fs.StringVar(&pushoverAPIToken, "pushover-api-token", "", "Pushover Application API Token (--kind pushover only, an alternative to --notify-url)")
	fs.BoolVar(&disabled, "disabled", false, "create the channel disabled (default: enabled)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if kind == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--kind is required"))
	}

	resolvedURL := notifyURL
	if resolvedURL == "" && kind == "pushover" && pushoverUserKey != "" && pushoverAPIToken != "" {
		resolvedURL = buildPushoverNotifyURL(pushoverUserKey, pushoverAPIToken)
	}
	if resolvedURL == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--notify-url is required (or, for --kind pushover, both --pushover-user-key and --pushover-api-token)"))
	}

	enabled := !disabled
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	created, err := client.CreateNotificationChannel(context.Background(), createNotificationChannelRequest{
		Name: name, Kind: kind, NotifyURL: resolvedURL, Enabled: &enabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create notification channel %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "channel %q (id %s, kind %s) connected\n", created.Name, created.ID, created.Kind)
	return exitOK
}

func channelsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels create --name NAME --kind KIND --notify-url URL [flags]
  %[1]s channels create --name NAME --kind pushover --pushover-user-key KEY --pushover-api-token TOKEN [flags]

Connects a new notification channel.

Flags:
  --name string                        display name for the channel (required)
  --kind string                        channel kind: generic, slack, discord, telegram, email, pushover (required)
  --notify-url string                  destination (webhook URL, Telegram sendMessage URL, or email address)
  --pushover-user-key string           Pushover User Key (--kind pushover only, an alternative to --notify-url)
  --pushover-api-token string          Pushover Application API Token (--kind pushover only, an alternative to --notify-url)
  --disabled                           create the channel disabled (default: enabled)
  --token string                       API token (default: %[2]s env var, then the credentials file)
  --api-url string                    control plane base URL (default: %[3]s env var, then %[4]s)
  --json                                 print the created channel as JSON to stdout, nothing else
  -h, --help                           show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
