package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runPITRBaseBackups dispatches "pitr base-backups <verb> <database>
// [flags]" to one of list/trigger, mirroring runBackupsSchedule's own
// set/clear dispatch shape for a different per-resource sub-config.
func runPITRBaseBackups(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, pitrBaseBackupsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, pitrBaseBackupsUsage(prog))
		return exitOK
	case "list":
		return runPITRBaseBackupsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "trigger":
		return runPITRBaseBackupsTrigger(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown pitr base-backups subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, pitrBaseBackupsUsage(prog))
		return exitUsage
	}
}

func pitrBaseBackupsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr base-backups list <database> [flags]                list physical base backup attempts
  %[1]s pitr base-backups trigger <database> --target ID [flags] trigger a manual physical base backup

Run "%[1]s pitr base-backups <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runPITRBaseBackupsList implements "pitr base-backups list
// <database>": GET /api/v1/databases/{name}/base-backups
// (internal/api/pitr.go's own handleListBaseBackupHistory), AbilityRead-
// gated.
func runPITRBaseBackupsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr base-backups list", "print base backup history as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pitrBaseBackupsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "pitr base-backups list", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	history, err := client.ListBaseBackups(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list base backups for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, history, func() { printBaseBackupHistoryTable(stdout, history) })
}

func printBaseBackupHistoryTable(out io.Writer, history []baseBackupHistoryResource) {
	if len(history) == 0 {
		_, _ = fmt.Fprintln(out, "no base backups")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTARGET\tSTATUS\tSIZE\tSTARTED\tFINISHED")
	for _, h := range history {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", h.ID, h.TargetID, h.Status, h.SizeBytes, h.StartedAt, h.FinishedAt)
	}
	_ = tw.Flush()
}

func pitrBaseBackupsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr base-backups list <database> [flags]

Lists <database>'s physical base backup attempt history, the input a
"pitr restore" targets by --base-backup.

Flags:
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print base backup history as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runPITRBaseBackupsTrigger implements "pitr base-backups trigger
// <database> --target ID": POST /api/v1/databases/{name}/base-backups
// (internal/api/pitr.go's own handleTriggerBaseBackup), AbilityWriteSensitive-
// gated. Returns 409 if PITR isn't enabled for the database yet.
func runPITRBaseBackupsTrigger(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr base-backups trigger", "print the started base backup attempt as JSON to stdout and nothing else", stderr)
	var targetID string
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pitrBaseBackupsTriggerUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "pitr base-backups trigger", "database name")
	if !ok {
		return exitUsage
	}

	if targetID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.TriggerBaseBackup(context.Background(), name, targetID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("trigger base backup for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stdout, "base backup %q for database %q started; check \"%s pitr base-backups list %s\" for status\n", started.ID, name, prog, name)
	})
}

func pitrBaseBackupsTriggerUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr base-backups trigger <database> --target ID [flags]

Starts a manual physical base backup of <database> (pg_basebackup) and
returns as soon as the attempt is recorded and under way. Requires PITR
already enabled ("pitr enable <database>").

Flags:
  --target string          backup target id to back up to (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started base backup attempt as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
