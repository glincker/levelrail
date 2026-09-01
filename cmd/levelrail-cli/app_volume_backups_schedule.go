package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppVolumeBackupsSchedule dispatches "app-volume-backups schedule
// <verb> <app> <volume> [flags]" to one of set/clear, mirroring
// runBackupsSchedule's exact dispatch shape (backups_schedule.go) for
// the volume resource kind.
func runAppVolumeBackupsSchedule(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appVolumeBackupsScheduleUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appVolumeBackupsScheduleUsage(prog))
		return exitOK
	case "set":
		return runAppVolumeBackupsScheduleSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppVolumeBackupsScheduleClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown app-volume-backups schedule subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appVolumeBackupsScheduleUsage(prog))
		return exitUsage
	}
}

func appVolumeBackupsScheduleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [flags]   configure a recurring backup
  %[1]s app-volume-backups schedule clear <app> <volume> [flags]                        remove a recurring backup

Run "%[1]s app-volume-backups schedule <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppVolumeBackupsScheduleSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "app-volume-backups schedule set", "print the saved schedule as JSON to stdout and nothing else", stderr)
	var targetID, cron string
	var retain, retainDays int
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (required)")
	fs.StringVar(&cron, "cron", "", "standard 5-field cron expression: minute hour day-of-month month day-of-week (required)")
	fs.IntVar(&retain, "retain", 0, "number of past backups to keep before older ones are deleted (0: no limit)")
	fs.IntVar(&retainDays, "retain-days", 0, "delete backups older than this many days (0: no limit), independent of --retain")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [flags]\n\nConfigures a recurring backup, replacing any previously configured\nschedule for <app>/<volume>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups schedule set", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	if targetID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target is required"))
	}
	if cron == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--cron is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	schedule, err := client.SetVolumeBackupSchedule(context.Background(), name, volume, setVolumeBackupScheduleRequest{
		TargetID:   targetID,
		Schedule:   cron,
		Retain:     retain,
		RetainDays: retainDays,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set backup schedule for %s/%s: %w", name, volume, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, schedule); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup schedule %q set for %s/%s (target %s, retain %d, retain_days %d)\n", schedule.Schedule, name, volume, schedule.TargetID, schedule.Retain, schedule.RetainDays)
	return exitOK
}

func runAppVolumeBackupsScheduleClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "app-volume-backups schedule clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s app-volume-backups schedule clear <app> <volume> [flags]\n\nRemoves <app>/<volume>'s recurring backup schedule. Past backup history is\nunaffected.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups schedule clear", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.ClearVolumeBackupSchedule(context.Background(), name, volume); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear backup schedule for %s/%s: %w", name, volume, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"cleared": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup schedule removed for %s/%s\n", name, volume)
	return exitOK
}
