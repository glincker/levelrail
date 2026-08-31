package main

import (
	"context"
	"flag"
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
	fs := flag.NewFlagSet(prog+" backups verifications", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, backupID string
	var jsonOut bool
	fs.StringVar(&backupID, "backup", "", "id of a backup to show verification history for (required)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print verification history as a JSON array to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsVerificationsUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups verifications requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	verifications, err := client.ListBackupVerifications(context.Background(), name, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list verifications for backup %q: %w", backupID, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, verifications); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printBackupVerificationsTable(stdout, verifications)
	return exitOK
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
  --json                     print verification history as a JSON array to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
