package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runAppsBackupsServices(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "apps backups services", "print the services as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBackupsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps backups services", "an app name", 1)
	if !ok {
		return exitUsage
	}
	app := rest[0]
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	items, err := client.ListComposeDBServices(context.Background(), app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list database services of %s: %w", app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, items, func() {
		if len(items) == 0 {
			_, _ = fmt.Fprintf(stdout, "no database services detected in %s\n", app)
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SERVICE\tENGINE\tSCHEDULE\tTARGET\tRETAIN\tENABLED")
		for _, s := range items {
			sched := "-"
			if s.Scheduled {
				sched = s.Schedule
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%t\n", s.Service, s.Engine, sched, dashIfEmpty(s.TargetID), s.Retain, s.Enabled)
		}
		_ = tw.Flush()
	})
}

func runAppsBackupsSchedule(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBackupsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "set":
		return runAppsBackupsScheduleSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsBackupsScheduleClear(prog, args[1:], stdout, stderr, lookupEnv)
	case "pause":
		return runAppsBackupsSchedulePause(prog, args[1:], true, stdout, stderr, lookupEnv)
	case "resume":
		return runAppsBackupsSchedulePause(prog, args[1:], false, stdout, stderr, lookupEnv)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBackupsUsage(prog))
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps backups schedule subcommand %q\n\n%s", prog, args[0], appsBackupsUsage(prog))
	return exitUsage
}

func runAppsBackupsScheduleSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "apps backups schedule set", "print the saved schedule as JSON to stdout and nothing else", stderr)
	var targetID, cron string
	var retain, retainDays int
	var disable bool
	fs.StringVar(&targetID, "target", "", "backup target id (required)")
	fs.StringVar(&cron, "cron", "", "5-field cron expression (required)")
	fs.IntVar(&retain, "retain", 0, "number of backups to keep (0: no limit)")
	fs.IntVar(&retainDays, "retain-days", 0, "delete backups older than this many days (0: no limit)")
	fs.BoolVar(&disable, "disable", false, "save the schedule but pause it")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBackupsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps backups schedule set", "an app name and a service name", 2)
	if !ok {
		return exitUsage
	}
	app, service := rest[0], rest[1]
	if targetID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target is required"))
	}
	if cron == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--cron is required"))
	}
	if retain < 0 || retainDays < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--retain and --retain-days must not be negative"))
	}
	req := apiclient.SetComposeDBBackupScheduleRequest{TargetID: targetID, Schedule: cron, Retain: retain, RetainDays: retainDays}
	if disable {
		off := false
		req.Enabled = &off
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	saved, err := client.SetComposeDBBackupSchedule(context.Background(), app, service, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set backup schedule for %s/%s: %w", app, service, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, saved, func() {
		_, _ = fmt.Fprintf(stdout, "backup schedule %q set for %s/%s (target %s, retain %d, retain_days %d, enabled %t)\n", saved.Schedule, app, service, saved.TargetID, saved.Retain, saved.RetainDays, saved.Enabled)
	})
}

func runAppsBackupsScheduleClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "apps backups schedule clear", "print {\"cleared\": true} as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBackupsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps backups schedule clear", "an app name and a service name", 2)
	if !ok {
		return exitUsage
	}
	app, service := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	if err := client.DeleteComposeDBBackupSchedule(context.Background(), app, service); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear backup schedule for %s/%s: %w", app, service, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "backup schedule removed for %s/%s; existing backup objects are kept\n", app, service)
	})
}

func runAppsBackupsSchedulePause(prog string, args []string, paused bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := pauseVerbName(paused)
	fs, tokenP, urlP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "apps backups schedule "+verb, "print the schedule as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBackupsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps backups schedule "+verb, "an app name and a service name", 2)
	if !ok {
		return exitUsage
	}
	app, service := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	saved, err := client.PauseComposeDBBackupSchedule(context.Background(), app, service, paused)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s backup schedule for %s/%s: %w", verb, app, service, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, saved, func() {
		_, _ = fmt.Fprintf(stdout, "backup schedule for %s/%s: %sd; the saved cron, target and retention are kept\n", app, service, verb)
	})
}

func runAppsBackupsRuns(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "apps backups runs", "print the runs as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBackupsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps backups runs", "an app name and a service name", 2)
	if !ok {
		return exitUsage
	}
	app, service := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	runs, err := client.ListComposeDBBackupRuns(context.Background(), app, service)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list backup runs of %s/%s: %w", app, service, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, runs, func() {
		if len(runs) == 0 {
			_, _ = fmt.Fprintf(stdout, "no backup runs for %s/%s\n", app, service)
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "RUN\tSTATUS\tSIZE\tSTARTED\tFINISHED\tERROR")
		for _, r := range runs {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", r.ID, r.Status, r.SizeBytes, r.StartedAt, dashIfEmpty(r.FinishedAt), dashIfEmpty(r.Error))
		}
		_ = tw.Flush()
	})
}

func runAppsBackupsRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "apps backups run", "print the started run as JSON to stdout and nothing else", stderr)
	var targetID string
	fs.StringVar(&targetID, "target", "", "backup target id (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBackupsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps backups run", "an app name and a service name", 2)
	if !ok {
		return exitUsage
	}
	app, service := rest[0], rest[1]
	if targetID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target is required"))
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	run, err := client.TriggerComposeDBBackup(context.Background(), app, service, targetID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start backup of %s/%s: %w", app, service, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, run, func() {
		_, _ = fmt.Fprintf(stdout, "backup of %s/%s started (run %s, status %s)\n", app, service, run.ID, run.Status)
	})
}
