package main

import (
	"context"
	"fmt"
	"io"
)

// runChannelsUpdate implements "channels update <id> --name NAME --kind
// KIND": PUT /api/v1/notification-channels/{id}, a full replace using
// the exact same flags "channels create" accepts. Every field the
// channel should keep must be passed again on this call, the same
// convention "registry-credentials update" already establishes: fixing a
// typo'd --notify-url no longer requires disconnecting and reconnecting
// the channel (which would also drop its delivery history).
func runChannelsUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "channels update", "print the updated channel as JSON to stdout and nothing else", stderr)
	var name, kind, notifyURL, pushoverUserKey, pushoverAPIToken, pagerDutyRoutingKey string
	var resendAPIKey, resendTo, resendFrom, opsgenieAPIKey string
	var disabled bool
	fs.StringVar(&name, "name", "", "display name for the channel (required)")
	fs.StringVar(&kind, "kind", "", "channel kind: generic, slack, discord, telegram, email, pushover, pagerduty, teams, resend, ntfy, gotify, mattermost, lark, rocketchat, opsgenie, webex, googlechat (required)")
	fs.StringVar(&notifyURL, "notify-url", "", "destination (webhook URL, Telegram sendMessage URL, email address, or PagerDuty routing key); required unless a kind-specific alternative below is set")
	fs.StringVar(&pushoverUserKey, "pushover-user-key", "", "Pushover User Key (--kind pushover only, an alternative to --notify-url)")
	fs.StringVar(&pushoverAPIToken, "pushover-api-token", "", "Pushover Application API Token (--kind pushover only, an alternative to --notify-url)")
	fs.StringVar(&pagerDutyRoutingKey, "pagerduty-routing-key", "", "PagerDuty Events API v2 Integration/Routing Key (--kind pagerduty only, an alternative to --notify-url)")
	fs.StringVar(&resendAPIKey, "resend-api-key", "", "Resend API Key (--kind resend only, an alternative to --notify-url)")
	fs.StringVar(&resendTo, "resend-to", "", "Resend destination email address (--kind resend only, required with --resend-api-key)")
	fs.StringVar(&resendFrom, "resend-from", "", "Resend from address (--kind resend only, optional, defaults to onboarding@resend.dev)")
	fs.StringVar(&opsgenieAPIKey, "opsgenie-api-key", "", "Opsgenie API Key (--kind opsgenie only, an alternative to --notify-url)")
	fs.BoolVar(&disabled, "disabled", false, "leave the channel disabled (default: enabled)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsUpdateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "channels update", "channel id")
	if !ok {
		return exitUsage
	}

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
	if resolvedURL == "" && kind == "resend" && resendAPIKey != "" && resendTo != "" {
		resolvedURL = buildResendNotifyURL(resendAPIKey, resendTo, resendFrom)
	}
	if resolvedURL == "" && kind == "opsgenie" && opsgenieAPIKey != "" {
		resolvedURL = buildOpsgenieNotifyURL(opsgenieAPIKey)
	}
	if resolvedURL == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--notify-url is required (or, for --kind pushover, both --pushover-user-key and --pushover-api-token; or, for --kind pagerduty, --pagerduty-routing-key; or, for --kind resend, both --resend-api-key and --resend-to; or, for --kind opsgenie, --opsgenie-api-key)"))
	}

	enabled := !disabled
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	updated, err := client.UpdateNotificationChannel(context.Background(), id, updateNotificationChannelRequest{
		Name: name, Kind: kind, NotifyURL: resolvedURL, Enabled: &enabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update notification channel %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stdout, "channel %q (id %s, kind %s) updated\n", updated.Name, updated.ID, updated.Kind)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func channelsUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels update <id> --name NAME --kind KIND --notify-url URL [flags]
  %[1]s channels update <id> --name NAME --kind pushover --pushover-user-key KEY --pushover-api-token TOKEN [flags]
  %[1]s channels update <id> --name NAME --kind pagerduty --pagerduty-routing-key KEY [flags]
  %[1]s channels update <id> --name NAME --kind resend --resend-api-key KEY --resend-to EMAIL [flags]
  %[1]s channels update <id> --name NAME --kind opsgenie --opsgenie-api-key KEY [flags]

Fully replaces an existing notification channel's configuration: every
flag you want kept must be passed again on this call, the same
convention "registry-credentials update" uses. Fixing a typo'd
--notify-url no longer requires disconnecting and reconnecting the
channel (which would also drop its recorded delivery history).

Flags:
  --name string                        display name for the channel (required)
  --kind string                        channel kind: generic, slack, discord, telegram, email, pushover, pagerduty, teams, resend, ntfy, gotify, mattermost, lark, rocketchat, opsgenie, webex, googlechat (required)
  --notify-url string                  destination (webhook URL, Telegram sendMessage URL, email address, or PagerDuty routing key)
  --pushover-user-key string           Pushover User Key (--kind pushover only, an alternative to --notify-url)
  --pushover-api-token string          Pushover Application API Token (--kind pushover only, an alternative to --notify-url)
  --pagerduty-routing-key string       PagerDuty Events API v2 Integration/Routing Key (--kind pagerduty only, an alternative to --notify-url)
  --resend-api-key string              Resend API Key (--kind resend only, an alternative to --notify-url)
  --resend-to string                   Resend destination email address (--kind resend only, required with --resend-api-key)
  --resend-from string                 Resend from address (--kind resend only, optional, defaults to onboarding@resend.dev)
  --opsgenie-api-key string            Opsgenie API Key (--kind opsgenie only, an alternative to --notify-url)
  --disabled                           leave the channel disabled (default: enabled)
  --token string                       API token (default: %[2]s env var, then the credentials file)
  --api-url string                    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string                    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                                 print the updated channel as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
