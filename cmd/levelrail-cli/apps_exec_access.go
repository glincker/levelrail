package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsExecAccess dispatches "apps exec-access <verb> [flags]" to one
// of enable/disable/status, the CLI counterpart of internal/api/exec.go's
// own GET/PUT /api/v1/apps/{name}/exec-access routes: whether
// handleExecApp/handleAppTerminal even attempt to reach this app's
// container, independent of whatever IAM abilities the caller's own
// token has. On by default, the opposite shape "apps auto-rollback
// enable/disable/status" establishes for its own opt-in feature: exec is
// available today with no equivalent gate, so this starts on for every
// app and an operator explicitly turns it off per app.
func runAppsExecAccess(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsExecAccessUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsExecAccessUsage(prog))
		return exitOK
	case "enable":
		return runAppsExecAccessSetEnabled(prog, args[1:], stdout, stderr, lookupEnv, true)
	case "disable":
		return runAppsExecAccessSetEnabled(prog, args[1:], stdout, stderr, lookupEnv, false)
	case "status":
		return runAppsExecAccessStatus(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps exec-access subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsExecAccessUsage(prog))
		return exitUsage
	}
}

func appsExecAccessUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps exec-access enable <app-name> [flags]    allow shell/exec access into this app
  %[1]s apps exec-access disable <app-name> [flags]   block shell/exec access into this app
  %[1]s apps exec-access status <app-name> [flags]    show whether it's currently on

On by default. When disabled, POST .../exec and the interactive terminal
(GET .../terminal) are both refused before ever attempting to reach the
container, regardless of what IAM abilities the calling token otherwise
has. Disable this on a production app to lock down shell access even for
an admin-level token; re-enabling it is an explicit, separate step.

Run "%[1]s apps exec-access <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsExecAccessSetEnabled(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), enabled bool) int {
	verb := "enable"
	if !enabled {
		verb = "disable"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps exec-access "+verb, "print the resulting setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsExecAccessSetEnabledUsage(prog, verb)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps exec-access "+verb, "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.SetExecAccess(context.Background(), appName, enabled)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s exec access for app %q: %w", verb, appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "exec access %sd for app %q\n", verb, appName)
	})
}

func appsExecAccessSetEnabledUsage(prog, verb string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps exec-access %[5]s <app-name> [flags]

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

func runAppsExecAccessStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps exec-access status", "print the current setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps exec-access status <app-name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps exec-access status", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.GetExecAccess(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get exec access status for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "exec access: %t\n", result.Enabled)
	})
}
