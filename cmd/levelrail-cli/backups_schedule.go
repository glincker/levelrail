package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupsSchedule dispatches "backups schedule <verb> <database>
// [flags]" to one of set/clear, mirroring runDomainsBasicAuth's own
// set/clear dispatch shape for a different per-resource sub-config.
func runBackupsSchedule(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, backupsScheduleUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, backupsScheduleUsage(prog))
		return exitOK
	case "set":
		return runBackupsScheduleSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runBackupsScheduleClear(prog, args[1:], stdout, stderr, lookupEnv)
	case "pause":
		return runBackupsSchedulePause(prog, args[1:], true, stdout, stderr, lookupEnv)
	case "resume":
		return runBackupsSchedulePause(prog, args[1:], false, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown backups schedule subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, backupsScheduleUsage(prog))
		return exitUsage
	}
}

func backupsScheduleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups schedule set <database> --target ID --cron EXPR [flags]   configure a recurring backup
  %[1]s backups schedule clear <database> [flags]                        remove a recurring backup
  %[1]s backups schedule pause <database> [flags]                        stop running it, keep the config
  %[1]s backups schedule resume <database> [flags]                       run it again

Run "%[1]s backups schedule <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runBackupsScheduleSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups schedule set", "print the saved schedule as JSON to stdout and nothing else", stderr)
	var targetID, cron string
	var retain, retainDays int
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (required)")
	fs.StringVar(&cron, "cron", "", "standard 5-field cron expression: minute hour day-of-month month day-of-week (required)")
	fs.IntVar(&retain, "retain", 0, "number of backups to keep (0: the server default, backups.schedule_retain_default)")
	fs.IntVar(&retainDays, "retain-days", 0, "delete backups older than this many days (0: no limit), independent of --retain")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s backups schedule set <database> --target ID --cron EXPR [flags]\n\nConfigures a recurring backup, replacing any previously configured\nschedule for <database>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "backups schedule set", "database name")
	if !ok {
		return exitUsage
	}
	if targetID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target is required"))
	}
	if cron == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--cron is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	schedule, err := client.SetBackupSchedule(context.Background(), name, setBackupScheduleRequest{
		TargetID:   targetID,
		Schedule:   cron,
		Retain:     retain,
		RetainDays: retainDays,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set backup schedule for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, schedule, func() {
		_, _ = fmt.Fprintf(stdout, "backup schedule %q set for database %q (target %s, retain %d, retain_days %d)\n", schedule.Schedule, name, schedule.TargetID, schedule.Retain, schedule.RetainDays)
	})
}

func runBackupsScheduleClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups schedule clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s backups schedule clear <database> [flags]\n\nRemoves <database>'s recurring backup schedule. Past backup history is\nunaffected.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "backups schedule clear", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.ClearBackupSchedule(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear backup schedule for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() { _, _ = fmt.Fprintf(stdout, "backup schedule removed for database %q\n", name) })
}

func runBackupsSchedulePause(prog string, args []string, paused bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := pauseVerbName(paused)
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups schedule "+verb, "print the schedule as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s backups schedule %s <database> [flags]\n\nThe saved cron, target and retention are kept; manual backups still work.\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "backups schedule "+verb, "database name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	schedule, err := client.PauseBackupSchedule(context.Background(), name, paused)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s backup schedule for database %q: %w", verb, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, schedule, func() {
		_, _ = fmt.Fprintf(stdout, "backup schedule for database %q: %s\n", name, verb+"d")
	})
}

func pauseVerbName(paused bool) string {
	if paused {
		return "pause"
	}
	return "resume"
}
