package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupsVerify implements "backups verify <database> --backup ID":
// POST /api/v1/databases/{name}/backups/{historyId}/verify
// (internal/api/backup_verify.go's own handleVerifyBackup),
// AbilityWriteSensitive-gated. Asynchronous, the same shape "backups
// trigger" already establishes: this returns as soon as the attempt is
// recorded and under way (202 Accepted server-side), not once the
// re-download and checks actually finish; use "backups verifications
// <database> --backup ID" to see whether it passed. Never attempts a live
// restore against a running database.
func runBackupsVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups verify", "print the started verification attempt as JSON to stdout and nothing else", stderr)
	var backupID string
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to verify (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsVerifyUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "backups verify", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	started, err := client.VerifyBackup(context.Background(), name, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("verify backup %q for database %q: %w", backupID, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stdout, "verification %q of backup %q started; check \"%s backups verifications %s --backup %s\" for status\n", started.ID, backupID, prog, name, backupID)
	})
}

func backupsVerifyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups verify <database> --backup ID [flags]

Re-downloads a previously succeeded backup and confirms it is still
intact (checksum, size, and a lightweight structural check), without ever
attempting a live restore against a running database. Returns as soon as
the attempt is recorded and under way. Check "%[1]s backups verifications
<database> --backup ID" to see whether it passed.

Flags:
  --backup string          id of a previously succeeded backup to verify (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started verification attempt as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
