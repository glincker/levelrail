package main

import (
	"context"
	"flag"
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

Run "%[1]s backups schedule <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runBackupsScheduleSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "backups schedule set", "print the saved schedule as JSON to stdout and nothing else", stderr)
	var targetID, cron string
	var retain, retainDays int
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (required)")
	fs.StringVar(&cron, "cron", "", "standard 5-field cron expression: minute hour day-of-month month day-of-week (required)")
	fs.IntVar(&retain, "retain", 0, "number of past backups to keep before older ones are deleted (0: no limit)")
	fs.IntVar(&retainDays, "retain-days", 0, "delete backups older than this many days (0: no limit), independent of --retain")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s backups schedule set <database> --target ID --cron EXPR [flags]\n\nConfigures a recurring backup, replacing any previously configured\nschedule for <database>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

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

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	schedule, err := client.SetBackupSchedule(context.Background(), name, setBackupScheduleRequest{
		TargetID:   targetID,
		Schedule:   cron,
		Retain:     retain,
		RetainDays: retainDays,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set backup schedule for database %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, schedule); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup schedule %q set for database %q (target %s, retain %d, retain_days %d)\n", schedule.Schedule, name, schedule.TargetID, schedule.Retain, schedule.RetainDays)
	return exitOK
}

func runBackupsScheduleClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" backups schedule clear", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s backups schedule clear <database> [flags]\n\nRemoves <database>'s recurring backup schedule. Past backup history is\nunaffected.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	name, ok := requireOneArg(fs, stderr, prog, "backups schedule clear", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	if err := client.ClearBackupSchedule(context.Background(), name); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("clear backup schedule for database %q: %w", name, err))
	}
	_, _ = fmt.Fprintf(stdout, "backup schedule removed for database %q\n", name)
	return exitOK
}
