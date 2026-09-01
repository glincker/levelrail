package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runBackupsVerifications implements "backups verifications <database>
// --backup ID": GET /api/v1/databases/{name}/backups/{historyId}/verifications
// (internal/api/backup_verify.go's own handleListBackupVerifications),
// AbilityRead-gated: passive visibility into verification attempts
// already made, the CLI counterpart of the backup history badge in the
// web dashboard's own backup history table.
func runBackupsVerifications(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups verifications", "print verification history as a JSON array to stdout and nothing else", stderr)
	var backupID string
	fs.StringVar(&backupID, "backup", "", "id of a backup to show verification history for (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsVerificationsUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "backups verifications", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	verifications, err := client.ListBackupVerifications(context.Background(), name, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list verifications for backup %q: %w", backupID, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, verifications, func() { printBackupVerificationsTable(stdout, verifications) })
}

// printBackupVerificationsTable prints a compact, aligned table of
// verification attempts, the same shape printBackupHistoryTable
// (backups_list.go) already establishes for backup attempts.
func printBackupVerificationsTable(out io.Writer, verifications []backupVerificationResource) {
	if len(verifications) == 0 {
		_, _ = fmt.Fprintln(out, "no verifications")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tCHECKSUM\tSIZE\tFORMAT\tCHECKED BY\tSTARTED\tFINISHED")
	for _, v := range verifications {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			v.ID, v.Status, checkMark(v.ChecksumMatch), checkMark(v.SizeMatch), checkMark(v.FormatValid), v.CheckedBy, v.StartedAt, v.FinishedAt)
	}
	_ = tw.Flush()
}

// checkMark renders a single verification check's pass/fail as a
// terminal-friendly "ok"/"FAIL" rather than a bare "true"/"false", the
// same "make the failing case visually louder" reasoning the web
// dashboard's own status badges follow with color instead.
func checkMark(ok bool) string {
	if ok {
		return "ok"
	}
	return "FAIL"
}

func backupsVerificationsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups verifications <database> --backup ID [flags]

Lists a backup's verification attempt history, newest first.

Flags:
  --backup string          id of a backup to show verification history for (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print verification history as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
