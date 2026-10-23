package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// runFunctionsControls handles "functions settings|versions|rollback".
func runFunctionsControls(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "functions "+verb, "print the result as JSON to stdout and nothing else", stderr)
	timeout := fs.Duration("timeout", 0, "request timeout while a sleeping function wakes, such as 45s; 0 restores the server default (settings)")
	concurrency := fs.Int("concurrency", 0, "most requests held at once while it wakes; 0 restores the server default (settings)")
	idle := fs.Duration("idle", 0, "idle time before the function sleeps, 5m to 168h (settings)")
	to := fs.String("to", "", "deploy ID to roll back to, default the newest earlier version (rollback)")
	pin := fs.Bool("pin", false, "also lock publishing so a new build cannot replace this version (rollback)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s functions %s <name> [flags]\n\nsettings: [--timeout 45s] [--concurrency 20] [--idle 10m]\nrollback: [--to <deploy id>] [--pin]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "functions "+verb, "function name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	switch verb {
	case "settings":
		return functionSettings(ctx, client, name, set, *timeout, *concurrency, *idle, of, stdout, stderr, jsonOut)
	case "versions":
		versions, err := client.ListFunctionVersions(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list versions of %q: %w", name, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, versions, func() {
			for _, v := range versions {
				mark := ""
				if v.Live {
					mark = " live"
				}
				_, _ = fmt.Fprintf(stdout, "%-14s %-10s %s  %s%s\n", shortID(v.DeployID), v.Status, v.StartedAt.Format(time.RFC3339), v.Image, mark)
			}
		})
	default:
		if err := client.RollbackFunction(ctx, name, *to, *pin); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("roll back %q: %w", name, err))
		}
		_, _ = fmt.Fprintf(stdout, "function %q rolled back\n", name)
		return exitOK
	}
}

func functionSettings(ctx context.Context, client *Client, name string, set map[string]bool, timeout time.Duration, concurrency int, idle time.Duration, of outputFlags, stdout, stderr io.Writer, jsonOut bool) int {
	if set["timeout"] || set["concurrency"] {
		var u apiclient.FunctionSettingsUpdate
		if set["timeout"] {
			s := int(timeout / time.Second)
			u.RequestTimeoutSeconds = &s
		}
		if set["concurrency"] {
			u.MaxConcurrency = &concurrency
		}
		if _, err := client.SetFunctionSettings(ctx, name, u); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update settings of %q: %w", name, err))
		}
	}
	if set["idle"] {
		if _, err := client.SetAppSleepHold(ctx, name, int(idle/time.Minute), true); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set idle period of %q: %w", name, err))
		}
	}
	s, err := client.GetFunctionSettings(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get settings of %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() {
		_, _ = fmt.Fprintf(stdout, "idle %dm\nport %d\nmemory %d bytes\ncpu %d nano\ntimeout %ds (effective %ds)\nconcurrency %d (effective %d)\n",
			s.IdleMinutes, s.Port, s.MemoryBytes, s.NanoCPUs, s.RequestTimeoutSeconds, s.EffectiveTimeoutSeconds, s.MaxConcurrency, s.EffectiveConcurrency)
	})
}
