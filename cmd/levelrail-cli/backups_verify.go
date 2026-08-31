package main

import (
	"context"
	"flag"
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
	fs := flag.NewFlagSet(prog+" backups verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, backupID string
	var jsonOut bool
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to verify (required)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print the started verification attempt as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsVerifyUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups verify requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	started, err := client.VerifyBackup(context.Background(), name, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("verify backup %q for database %q: %w", backupID, name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "verification %q of backup %q started; check \"%s backups verifications %s --backup %s\" for status\n", started.ID, backupID, prog, name, backupID)
	return exitOK
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
  --json                     print the started verification attempt as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
