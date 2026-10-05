package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSettingsDeployFreeze dispatches "settings deploy-freeze show|set|clear",
// the fleet-wide sibling of "apps freeze" (apps_freeze.go), backed by
// GET/PUT /api/v1/settings/deploy-freeze (internal/api/deploy_safety.go).
func runSettingsDeployFreeze(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsDeployFreezeUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsDeployFreezeUsage(prog))
		return exitOK
	case "show", "set", "clear":
		return runSettingsDeployFreezeVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings deploy-freeze subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsDeployFreezeUsage(prog))
		return exitUsage
	}
}

func settingsDeployFreezeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings deploy-freeze show [flags]
  %[1]s settings deploy-freeze set --cron EXPR --duration D [--timezone TZ] [--reason TEXT] [flags]
  %[1]s settings deploy-freeze clear [flags]

Fleet-wide deploy freeze windows: every app's own windows (see
"apps freeze") inherit these on top. A freeze window starts at every
match of a 5-field cron expression, evaluated in --timezone (default
UTC), and lasts --duration. While one is active, webhook, pipeline and
released deploys are held fleet-wide and run automatically when the
window ends; manual deploys need
"apps deploy --override-freeze --override-reason TEXT". "set" replaces
the fleet-wide windows with the one given.

Example, a weekend freeze in Berlin:
  %[1]s settings deploy-freeze set --cron "0 17 * * 5" --duration 64h --timezone Europe/Berlin --reason weekend

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read
  --json                    print the result as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runSettingsDeployFreezeVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "settings deploy-freeze " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the freeze state as JSON to stdout and nothing else", stderr)
	var win apiclient.FreezeWindowResource
	if verb == "set" {
		fs.StringVar(&win.Cron, "cron", "", "5-field cron expression for when each window starts (required)")
		fs.StringVar(&win.Duration, "duration", "", "how long each window lasts, e.g. 2h or 64h (required)")
		fs.StringVar(&win.Timezone, "timezone", "UTC", "IANA timezone the cron expression is evaluated in")
		fs.StringVar(&win.Reason, "reason", "", "shown on held deploys")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsDeployFreezeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if verb == "set" && (win.Cron == "" || win.Duration == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("--cron and --duration are required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		res apiclient.DeployFreezeResource
		err error
	)
	switch verb {
	case "show":
		res, err = client.GetGlobalDeployFreeze(ctx)
	case "set":
		res, err = client.SetGlobalDeployFreeze(ctx, []apiclient.FreezeWindowResource{win})
	default:
		res, err = client.SetGlobalDeployFreeze(ctx, nil)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDeployFreezeHuman(stdout, res) })
}
