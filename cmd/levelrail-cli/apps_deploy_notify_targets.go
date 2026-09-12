package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsDeployNotifyTargets dispatches "apps deploy-notify-targets <verb>
// [flags]" to one of list/create/delete, the CLI counterpart of
// internal/api/deploy_notify_targets.go's own
// /api/v1/apps/{name}/deploy-notify-targets routes.
func runAppsDeployNotifyTargets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsDeployNotifyTargetsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsDeployNotifyTargetsUsage(prog))
		return exitOK
	case "list":
		return runAppsDeployNotifyTargetsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runAppsDeployNotifyTargetsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsDeployNotifyTargetsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps deploy-notify-targets subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsDeployNotifyTargetsUsage(prog))
		return exitUsage
	}
}

func appsDeployNotifyTargetsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy-notify-targets list <app> [flags]
  %[1]s apps deploy-notify-targets create <app> --channel-id ID [flags]
  %[1]s apps deploy-notify-targets delete <app> <id> [flags]

Manages an app's deploy-outcome notification targets: each one attaches
an already-connected notification channel (see "channels list") so a
deploy success or failure sends a message there.

Run "%[1]s apps deploy-notify-targets <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsDeployNotifyTargetsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploy-notify-targets list", "print targets as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeployNotifyTargetsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps deploy-notify-targets list", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	targets, err := client.ListDeployNotifyTargets(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deploy notify targets for app %q: %w", appName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, targets, func() { printDeployNotifyTargetsTable(stdout, targets) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printDeployNotifyTargetsTable(out io.Writer, targets []deployNotifyTargetResource) {
	if len(targets) == 0 {
		_, _ = fmt.Fprintln(out, "no deploy notify targets")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tCHANNEL_ID\tKIND\tENABLED")
	for _, t := range targets {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\n", t.ID, t.ChannelID, t.NotifyKind, t.Enabled)
	}
	_ = tw.Flush()
}

func appsDeployNotifyTargetsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy-notify-targets list <app> [flags]

Lists an app's deploy-outcome notification targets, including disabled ones.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print targets as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsDeployNotifyTargetsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploy-notify-targets create", "print the created target as JSON to stdout and nothing else", stderr)
	var channelID string
	var disabled bool
	fs.StringVar(&channelID, "channel-id", "", "an already-connected notification channel to attach (required, see \"channels list\")")
	fs.BoolVar(&disabled, "disabled", false, "create the target disabled (default: enabled)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeployNotifyTargetsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps deploy-notify-targets create", "app name")
	if !ok {
		return exitUsage
	}

	if channelID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--channel-id is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateDeployNotifyTarget(context.Background(), appName, createDeployNotifyTargetRequest{
		ChannelID: channelID,
		Enabled:   !disabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create deploy notify target for app %q: %w", appName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "deploy notify target %q created for app %q (channel %q)\n", created.ID, appName, created.ChannelID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsDeployNotifyTargetsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy-notify-targets create <app> --channel-id ID [flags]

Creates a new deploy-outcome notification target for <app>, attaching an
already-connected notification channel (see "channels list").

Flags:
  --channel-id string      an already-connected notification channel to attach (required)
  --disabled                create the target disabled (default: enabled)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the created target as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsDeployNotifyTargetsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploy-notify-targets delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeployNotifyTargetsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, argsOK := requireArgs(fs, stderr, prog, "apps deploy-notify-targets delete", "an app name and a target id", 2)
	if !argsOK {
		return exitUsage
	}
	appName, id := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteDeployNotifyTarget(context.Background(), appName, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete deploy notify target %q for app %q: %w", id, appName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "deploy notify target %q deleted\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsDeployNotifyTargetsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy-notify-targets delete <app> <id> [flags]

Deletes one of <app>'s deploy-outcome notification targets.

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
