package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsSchedule dispatches "apps schedule set|get|history <app> [flags]".
func runAppsSchedule(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsScheduleUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsScheduleUsage(prog))
		return exitOK
	case "set":
		return runAppsScheduleSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runAppsScheduleGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runAppsScheduleList(prog, args[1:], stdout, stderr, lookupEnv)
	case "cancel":
		return runAppsScheduleCancel(prog, args[1:], stdout, stderr, lookupEnv)
	case "history":
		return runAppsScheduleHistory(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps schedule subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsScheduleUsage(prog))
		return exitUsage
	}
}

func appsScheduleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps schedule set <app> --cron EXPR --branch NAME [flags]   configure a recurring redeploy
  %[1]s apps schedule set <app> --run-at WHEN --branch NAME [flags] configure a one-time redeploy
  %[1]s apps schedule get <app> [flags]                             show the configured schedule
  %[1]s apps schedule history <app> [flags]                         show recent schedule evaluations
  %[1]s apps schedule list <app> [--status S] [flags]               list one-shot scheduled deploys (see "apps deploy --at")
  %[1]s apps schedule cancel <app> <id> [flags]                     cancel a pending one-shot scheduled deploy

Run "%[1]s apps schedule <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsScheduleSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps schedule set", "print the saved schedule as JSON to stdout and nothing else", stderr)
	var cron, runAt, branch, timezone string
	var disable bool
	fs.StringVar(&cron, "cron", "", "standard 5-field cron expression: minute hour day-of-month month day-of-week (one of --cron or --run-at is required)")
	fs.StringVar(&runAt, "run-at", "", "deploy once at an RFC 3339 timestamp or in a relative duration such as 2h or 30m (mutually exclusive with --cron)")
	fs.StringVar(&branch, "branch", "", "branch to redeploy the latest commit of when the schedule fires (required)")
	fs.StringVar(&timezone, "timezone", "UTC", "IANA timezone the cron expression is evaluated in")
	fs.BoolVar(&disable, "disable", false, "save the schedule disabled instead of enabled")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps schedule set <app> --cron EXPR --branch NAME [flags]\n\nConfigures <app>'s recurring (--cron) or one-time (--run-at) redeploy, replacing any previously\nconfigured schedule. Requires a connected git source (apps git-source).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps schedule set", "an app name", 1)
	if !ok {
		return exitUsage
	}
	name := rest[0]

	if cron != "" && runAt != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--cron and --run-at are mutually exclusive"))
	}
	if cron == "" && runAt == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("one of --cron or --run-at is required"))
	}
	var runAtTS string
	if runAt != "" {
		at, err := parseRunAt(runAt, time.Now())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("--run-at: %v", err))
		}
		runAtTS = at.Format(time.RFC3339)
	}
	if branch == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--branch is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	enabled := !disable
	schedule, err := client.SetAppSchedule(context.Background(), name, apiclient.SetAppScheduleRequest{
		Cron: cron, RunAt: runAtTS, Branch: branch, Timezone: timezone, Enabled: &enabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set schedule for %s: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, schedule, func() {
		if runAtTS != "" {
			_, _ = fmt.Fprintf(stdout, "one-time deploy set for %s at %s (branch %s, enabled %v)\n", name, runAtTS, schedule.Branch, schedule.Enabled)
			return
		}
		_, _ = fmt.Fprintf(stdout, "schedule %q set for %s (branch %s, timezone %s, enabled %v)\n", schedule.Cron, name, schedule.Branch, schedule.Timezone, schedule.Enabled)
	})
}

func runAppsScheduleGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps schedule get", "print the configured schedule as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps schedule get <app> [flags]\n\nShows <app>'s configured redeploy schedule.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps schedule get", "an app name", 1)
	if !ok {
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	schedule, err := client.GetAppSchedule(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get schedule for %s: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, schedule, func() {
		next := "not yet armed"
		if schedule.NextRunAt != nil {
			next = schedule.NextRunAt.Local().Format("2006-01-02 15:04:05 MST")
		}
		if schedule.RunAt != nil {
			state := "pending, runs at " + schedule.RunAt.Local().Format("2006-01-02 15:04:05 MST")
			if schedule.CompletedAt != nil {
				state = "completed at " + schedule.CompletedAt.Local().Format("2006-01-02 15:04:05 MST")
			}
			_, _ = fmt.Fprintf(stdout, "%s: one-time deploy %s, branch %s, enabled %v\n", name, state, schedule.Branch, schedule.Enabled)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s: cron %q, branch %s, timezone %s, enabled %v, next run %s\n", name, schedule.Cron, schedule.Branch, schedule.Timezone, schedule.Enabled, next)
	})
}

func runAppsScheduleHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps schedule history", "print the schedule history as JSON to stdout and nothing else", stderr)
	var limit int
	fs.IntVar(&limit, "limit", 0, "maximum number of entries to return (server default/max apply when 0)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps schedule history <app> [flags]\n\nShows <app>'s most recent schedule evaluations, newest first: whether\neach fired, was skipped for an active freeze window, or failed.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps schedule history", "an app name", 1)
	if !ok {
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	entries, err := client.ListAppScheduleHistory(context.Background(), name, limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list schedule history for %s: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, entries, func() {
		if len(entries) == 0 {
			_, _ = fmt.Fprintf(stdout, "no schedule history for %s\n", name)
			return
		}
		for _, e := range entries {
			line := fmt.Sprintf("%s  %-14s  scheduled for %s", e.FiredAt.Local().Format("2006-01-02 15:04:05"), e.Status, e.ScheduledFor.Local().Format("2006-01-02 15:04:05"))
			if e.Reason != "" {
				line += "  (" + e.Reason + ")"
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
	})
}

// parseRunAt accepts an RFC 3339 timestamp or a positive relative duration
// ("2h", "30m") and returns the absolute time, so the server only ever sees
// a timestamp and the caller can print it back.
func parseRunAt(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "in "))
	if at, err := time.Parse(time.RFC3339, raw); err == nil {
		return at, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is neither an RFC 3339 timestamp nor a duration like 2h or 30m", raw)
	}
	if d <= 0 {
		return time.Time{}, fmt.Errorf("relative duration %q must be positive", raw)
	}
	return now.Add(d).UTC().Truncate(time.Second), nil
}
