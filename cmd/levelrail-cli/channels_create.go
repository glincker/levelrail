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
// chat_id, an email address, a PagerDuty routing key, ...); --kind
// pushover additionally accepts --pushover-user-key/--pushover-api-token,
// and --kind pagerduty accepts --pagerduty-routing-key, both convenience
// aliases so an operator doesn't have to know the API expects them in
// --notify-url (mirroring the web dashboard's connect dialog fields).
func runChannelsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "channels create", "print the created channel as JSON to stdout and nothing else", stderr)
	var name, kind, notifyURL, pushoverUserKey, pushoverAPIToken, pagerDutyRoutingKey string
	var disabled bool
	fs.StringVar(&name, "name", "", "display name for the channel (required)")
	fs.StringVar(&kind, "kind", "", "channel kind: generic, slack, discord, telegram, email, pushover, pagerduty, teams (required)")
	fs.StringVar(&notifyURL, "notify-url", "", "destination (webhook URL, Telegram sendMessage URL, email address, or PagerDuty routing key); required unless a kind-specific alternative below is set")
	fs.StringVar(&pushoverUserKey, "pushover-user-key", "", "Pushover User Key (--kind pushover only, an alternative to --notify-url)")
	fs.StringVar(&pushoverAPIToken, "pushover-api-token", "", "Pushover Application API Token (--kind pushover only, an alternative to --notify-url)")
	fs.StringVar(&pagerDutyRoutingKey, "pagerduty-routing-key", "", "PagerDuty Events API v2 Integration/Routing Key (--kind pagerduty only, an alternative to --notify-url)")
	fs.BoolVar(&disabled, "disabled", false, "create the channel disabled (default: enabled)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

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
	if resolvedURL == "" && kind == "pagerduty" && pagerDutyRoutingKey != "" {
		resolvedURL = pagerDutyRoutingKey
	}
	if resolvedURL == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--notify-url is required (or, for --kind pushover, both --pushover-user-key and --pushover-api-token; or, for --kind pagerduty, --pagerduty-routing-key)"))
	}

	enabled := !disabled
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

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
  %[1]s channels create --name NAME --kind pagerduty --pagerduty-routing-key KEY [flags]

Connects a new notification channel.

Flags:
  --name string                        display name for the channel (required)
  --kind string                        channel kind: generic, slack, discord, telegram, email, pushover, pagerduty, teams (required)
  --notify-url string                  destination (webhook URL, Telegram sendMessage URL, email address, or PagerDuty routing key)
  --pushover-user-key string           Pushover User Key (--kind pushover only, an alternative to --notify-url)
  --pushover-api-token string          Pushover Application API Token (--kind pushover only, an alternative to --notify-url)
  --pagerduty-routing-key string       PagerDuty Events API v2 Integration/Routing Key (--kind pagerduty only, an alternative to --notify-url)
  --disabled                           create the channel disabled (default: enabled)
  --token string                       API token (default: %[2]s env var, then the credentials file)
  --api-url string                    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string                    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                                 print the created channel as JSON to stdout, nothing else
  -h, --help                           show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
