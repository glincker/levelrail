package main

import (
	"context"
	"fmt"
	"io"
)

// validSLOBurnAutoRollbackModes mirrors internal/api's own
// validSLOBurnAutoRollbackModes (deploys.go): the server validates too,
// this just gives a usage error before making a request for an obviously
// wrong value.
var validSLOBurnAutoRollbackModes = map[string]bool{
	"off": true, "auto": true, "dry_run": true, "pause_for_human": true,
}

// runAppsAutoRollbackSLOBurn dispatches "apps auto-rollback-slo-burn
// <verb> [flags]" to one of set/status, the CLI counterpart of
// internal/api/deploys.go's own GET/PUT
// /api/v1/apps/{name}/auto-rollback-slo-burn routes: how an app reacts
// the next time a kind=slo_burn alert rule fires for it. "off" by
// default, mirroring "apps auto-rollback" (apps_auto_rollback.go) but a
// set <mode> verb rather than enable/disable, since there are four modes
// to choose between, not two.
func runAppsAutoRollbackSLOBurn(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsAutoRollbackSLOBurnUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsAutoRollbackSLOBurnUsage(prog))
		return exitOK
	case "set":
		return runAppsAutoRollbackSLOBurnSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runAppsAutoRollbackSLOBurnStatus(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps auto-rollback-slo-burn subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsAutoRollbackSLOBurnUsage(prog))
		return exitUsage
	}
}

func appsAutoRollbackSLOBurnUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps auto-rollback-slo-burn set <app-name> <mode> [flags]   set how this app reacts to an SLO burn alert
  %[1]s apps auto-rollback-slo-burn status <app-name> [flags]       show the current mode

mode is one of:
  off               only alert, never act automatically (default)
  auto              roll back immediately to the most recent successful image
                    older than the one burning the error budget
  dry_run           log what would have been rolled back, never deploy
  pause_for_human   open a pending deploy approval instead of deploying
                    directly ("%[1]s deploy-approvals list")

Once set to auto or dry_run or pause_for_human, the next time a
kind=slo_burn alert rule fires for this app (see "apps alerts create
--kind slo_burn"), the control plane acts according to the chosen mode.
Fires at most once per firing episode; if there is no older successful
image to fall back to, the app is left to the alert alone.

Run "%[1]s apps auto-rollback-slo-burn <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsAutoRollbackSLOBurnSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps auto-rollback-slo-burn set", "print the resulting setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsAutoRollbackSLOBurnSetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps auto-rollback-slo-burn set", "an app name and a mode", 2)
	if !ok {
		return exitUsage
	}
	appName, mode := rest[0], rest[1]
	if !validSLOBurnAutoRollbackModes[mode] {
		_, _ = fmt.Fprintf(stderr, "%s: mode must be one of: off, auto, dry_run, pause_for_human\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.SetAutoRollbackSLOBurn(context.Background(), appName, mode)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set auto-rollback-slo-burn for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "auto-rollback-slo-burn for app %q set to %q\n", appName, mode)
	})
}

func appsAutoRollbackSLOBurnSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps auto-rollback-slo-burn set <app-name> <mode> [flags]

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the resulting setting as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsAutoRollbackSLOBurnStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps auto-rollback-slo-burn status", "print the current setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps auto-rollback-slo-burn status <app-name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "apps auto-rollback-slo-burn status", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.GetAutoRollbackSLOBurn(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get auto-rollback-slo-burn status for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "auto-rollback-slo-burn: %s\n", result.Mode)
	})
}
