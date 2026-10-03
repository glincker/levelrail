package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runPushSubscriptions dispatches "push-subscriptions <verb> [flags]" to
// list or revoke: the admin-side view of browser push registrations
// (Settings -> Notification channels -> Browser push in the web UI), the
// delivery target for the "webpush" notification-channel kind. No
// "register" subcommand here: a subscription's endpoint/keys come from
// the browser's own Push API, not something a CLI session can produce.
func runPushSubscriptions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, pushSubscriptionsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, pushSubscriptionsUsage(prog))
		return exitOK
	case "list":
		return runPushSubscriptionsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "revoke":
		return runPushSubscriptionsRevoke(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown push-subscriptions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, pushSubscriptionsUsage(prog))
		return exitUsage
	}
}

func pushSubscriptionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s push-subscriptions list [flags]           list registered browser push subscriptions
  %[1]s push-subscriptions revoke <id> [flags]    revoke a registered browser

Run "%[1]s push-subscriptions <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runPushSubscriptionsList implements "push-subscriptions list": GET
// /api/v1/settings/push-subscriptions.
func runPushSubscriptionsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "push-subscriptions list", "print subscriptions as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pushSubscriptionsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	subs, err := client.ListPushSubscriptions(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list push subscriptions: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, subs, func() { printPushSubscriptionsTable(stdout, subs) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printPushSubscriptionsTable(out io.Writer, subs []pushSubscriptionResource) {
	if len(subs) == 0 {
		_, _ = fmt.Fprintln(out, "no registered browser push subscriptions")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tUSER AGENT\tCREATED")
	for _, s := range subs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ID, s.UserAgent, s.CreatedAt)
	}
	_ = tw.Flush()
}

func pushSubscriptionsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s push-subscriptions list [flags]

Lists every registered browser push subscription for the caller's own account.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print subscriptions as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runPushSubscriptionsRevoke implements "push-subscriptions revoke <id>":
// DELETE /api/v1/settings/push-subscriptions/{id}.
func runPushSubscriptionsRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "push-subscriptions revoke", "print {\"revoked\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pushSubscriptionsRevokeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "push-subscriptions revoke", "subscription id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeletePushSubscription(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke push subscription %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"revoked": true}, func() {
		_, _ = fmt.Fprintf(stdout, "push subscription %q revoked\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func pushSubscriptionsRevokeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s push-subscriptions revoke <id> [flags]

Revokes one registered browser push subscription.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"revoked": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
