package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsSettingsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps settings predeploy-snapshot <app-name> [--mode off|on|require] [--target ID] [flags]

With no --mode, prints the current policy. Modes:
  off       never snapshot (default)
  on        snapshot the app's linked databases before each deploy; a failed
            snapshot is logged and the deploy goes ahead
  require   a failed snapshot blocks the deploy

--target picks the backup target; without it the only configured target is used.
Roll back with the data using "%[1]s apps rollback <app-name> --with-data".
`, prog)
}

// runAppsSettings dispatches "apps settings <topic>".
func runAppsSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsSettingsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsSettingsUsage(prog))
		return exitOK
	case "predeploy-snapshot":
		return runAppsPredeploySnapshot(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps settings topic %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsSettingsUsage(prog))
		return exitUsage
	}
}

func runAppsPredeploySnapshot(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps settings predeploy-snapshot", "print the policy as JSON to stdout and nothing else", stderr)
	var mode, target string
	fs.StringVar(&mode, "mode", "", "off, on or require (omit to show the current policy)")
	fs.StringVar(&target, "target", "", "backup target id (default: keep the current one)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsSettingsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps settings predeploy-snapshot", "app name")
	if !ok {
		return exitUsage
	}
	if mode != "" && mode != "off" && mode != "on" && mode != "require" {
		_, _ = fmt.Fprintf(stderr, "%s: --mode must be one of: off, on, require\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	current, err := client.GetPredeploySnapshotSettings(ctx, appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get predeploy snapshot policy for app %q: %w", appName, err))
	}
	result := current
	if mode != "" || target != "" {
		next := apiclient.PredeploySnapshotSettings{Mode: current.Mode, TargetID: current.TargetID}
		if mode != "" {
			next.Mode = mode
		}
		if target != "" {
			next.TargetID = target
		}
		if result, err = client.SetPredeploySnapshotSettings(ctx, appName, next); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set predeploy snapshot policy for app %q: %w", appName, err))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "predeploy-snapshot for app %q: mode %s, target %q\n", appName, result.Mode, result.TargetID)
	})
}

// takeBoolFlag removes every occurrence of name from args and reports whether one was present.
func takeBoolFlag(args []string, name string) ([]string, bool) {
	out := make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if a == name {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

// runAppsRollbackWithData restores the databases snapshotted before a deploy as new databases,
// swaps the app's links to them and redeploys the previous release.
func runAppsRollbackWithData(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps rollback --with-data", "print the operation as JSON to stdout and nothing else", stderr)
	var deployID string
	fs.StringVar(&deployID, "deploy", "", "the deploy to undo (default: the newest deploy that has a usable snapshot)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRollbackUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps rollback --with-data", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if deployID == "" {
		snaps, err := client.ListPredeploySnapshots(ctx, appName)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list predeploy snapshots for app %q: %w", appName, err))
		}
		for _, s := range snaps {
			if s.Status == "succeeded" && s.DeployID != "" {
				deployID = s.DeployID
				break
			}
		}
		if deployID == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("app %q has no pre-deploy snapshot to roll back to; enable one with \"apps settings predeploy-snapshot\"", appName))
		}
	}
	result, err := client.RollbackWithData(ctx, appName, deployID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("roll back app %q with data: %w", appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "app %q rolling back to deploy %s with data restored into new databases (%d); the old databases are untouched\n", appName, result.RollbackTo, len(result.Snapshots))
	})
}
