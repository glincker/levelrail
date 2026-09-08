package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupsDelete implements "backups delete <database> --backup ID":
// DELETE /api/v1/databases/{name}/backups/{historyId}
// (internal/api/backups.go's own handleDeleteBackupHistory),
// AbilityWriteSensitive-gated. Removes one backup attempt from history
// on request, any status, independent of the retention policy "backups
// schedule" configures.
func runBackupsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	var backupID string
	fs.StringVar(&backupID, "backup", "", "id of the backup attempt to delete (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsDeleteUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "backups delete", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	if err := client.DeleteBackupHistory(context.Background(), name, backupID); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete backup %q for database %q: %w", backupID, name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "backup %q deleted\n", backupID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func backupsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups delete <database> --backup ID [flags]

Removes one backup attempt from history, any status, independent of the
retention policy "%[1]s backups schedule" configures. If a backup target
runner is configured, the underlying bucket object is best-effort deleted
too; a failure there never blocks the history entry from being removed.

Flags:
  --backup string          id of the backup attempt to delete (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print {"deleted": true} as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
