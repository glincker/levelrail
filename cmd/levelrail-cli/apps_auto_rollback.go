package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsAutoRollback dispatches "apps auto-rollback <verb> [flags]" to
// one of enable/disable/status, the CLI counterpart of
// internal/api/deploys.go's own GET/PUT /api/v1/apps/{name}/auto-rollback
// routes: whether internal/alerting.MaybeAutoRollback rolls an app back
// automatically the next time a crashloop alert rule fires for it. Off
// by default, the same opt-in shape "apps previews enable/disable"
// already establishes.
func runAppsAutoRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsAutoRollbackUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsAutoRollbackUsage(prog))
		return exitOK
	case "enable":
		return runAppsAutoRollbackSetEnabled(prog, args[1:], stdout, stderr, lookupEnv, true)
	case "disable":
		return runAppsAutoRollbackSetEnabled(prog, args[1:], stdout, stderr, lookupEnv, false)
	case "status":
		return runAppsAutoRollbackStatus(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps auto-rollback subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsAutoRollbackUsage(prog))
		return exitUsage
	}
}

func appsAutoRollbackUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps auto-rollback enable <app-name> [flags]    opt an app into automatic rollback on a crashloop
  %[1]s apps auto-rollback disable <app-name> [flags]   opt an app back out
  %[1]s apps auto-rollback status <app-name> [flags]    show whether it's currently on

Off by default. Once enabled, the next time a kind=crashloop alert rule
fires for this app (see "apps alerts create --kind crashloop"), the
control plane automatically redeploys the most recent successful image
older than the one that's crashlooping, through the exact same trigger
path "apps rollback"/"apps deploy" use. Fires at most once per crashloop
episode; if there is no older successful image to fall back to, the app
is left to the crashloop alert alone rather than rolling back to nothing.

Run "%[1]s apps auto-rollback <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsAutoRollbackSetEnabled(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), enabled bool) int {
	verb := "enable"
	if !enabled {
		verb = "disable"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps auto-rollback "+verb, "print the resulting setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsAutoRollbackSetEnabledUsage(prog, verb)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps auto-rollback "+verb, "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.SetAutoRollback(context.Background(), appName, enabled)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s auto-rollback for app %q: %w", verb, appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "auto-rollback %sd for app %q\n", verb, appName)
	})
}

func appsAutoRollbackSetEnabledUsage(prog, verb string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps auto-rollback %[5]s <app-name> [flags]

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the resulting setting as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, verb)
}

func runAppsAutoRollbackStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps auto-rollback status", "print the current setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps auto-rollback status <app-name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps auto-rollback status", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.GetAutoRollback(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get auto-rollback status for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "auto-rollback: %t\n", result.Enabled)
	})
}
