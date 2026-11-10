package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsRestartScheduleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps restart-schedule <app-name> [flags]
  %[1]s apps restart-schedule <app-name> --schedule EXPR [--timezone ZONE] [flags]
  %[1]s apps restart-schedule <app-name> --clear [flags]

Restarts the app on a schedule through the normal restart path, so
rollout policy and disruption budgets apply. EXPR is a 5 field cron
("0 3 * * *") or "@every 24h", at least the minimum interval apart.
Stopped apps, apps in a protected environment, during a deploy freeze
or with a running deploy are skipped, never forced or started.

Flags:
  --schedule string       cron expression or "@every <duration>"
  --timezone string       IANA zone for a cron schedule (default UTC)
  --clear                 remove the schedule
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read
  --json                  print the result as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

// runAppsRestartSchedule implements "apps restart-schedule".
func runAppsRestartSchedule(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps restart-schedule"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var schedule, timezone string
	var clearFlag bool
	fs.StringVar(&schedule, "schedule", "", "cron expression or @every <duration>")
	fs.StringVar(&timezone, "timezone", "", "IANA zone for a cron schedule")
	fs.BoolVar(&clearFlag, "clear", false, "remove the schedule")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRestartScheduleUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	schedule = strings.TrimSpace(schedule)
	if clearFlag && (schedule != "" || timezone != "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: --clear cannot be combined with --schedule or --timezone", label))
	}
	if timezone != "" && schedule == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: --timezone needs --schedule", label))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.AppRestartSchedule
	var err error
	switch {
	case clearFlag:
		res, err = client.ClearAppRestartSchedule(ctx, name)
	case schedule != "":
		res, err = client.SetAppRestartSchedule(ctx, name, schedule, timezone)
	default:
		res, err = client.GetAppRestartSchedule(ctx, name)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s %q: %w", label, name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if res.Schedule == nil {
			_, _ = fmt.Fprintf(stdout, "app %q has no restart schedule (minimum interval %s)\n", name, res.MinInterval)
			return
		}
		s := res.Schedule
		_, _ = fmt.Fprintf(stdout, "app %q restarts on %q (%s), next at %s\n", name, s.Schedule, s.Timezone, s.NextRunAt)
		if s.LastStatus != "" {
			line := fmt.Sprintf("last run %s: %s", s.LastRunAt, s.LastStatus)
			if s.LastReason != "" {
				line += " (" + s.LastReason + ")"
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
