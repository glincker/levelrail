package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupsDelete implements "backups delete <database> <backup-id>":
// DELETE /api/v1/databases/{name}/backups/{historyId}
// (internal/api/backup_delete.go's own handleDeleteBackup),
// AbilityWriteSensitive-gated server-side, the same tier
// "backup-targets delete" already uses. That command's own precedent is
// this one's model: naming the exact resource id is itself the
// deliberate, unambiguous signal this destroys, the same reasoning
// backup_targets_delete.go's own doc comment gives, rather than adding a
// second --confirm flag on top of an id the caller already had to look up
// and type correctly. AbilityRoot routes (restore) ask for more because
// they act on a *name* an operator could otherwise fat-finger into the
// wrong live resource; a backup id has no such ambiguity to guard against.
func runBackupsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: backups delete requires a database name and a backup id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, backupID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteBackup(context.Background(), name, backupID); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete backup %q of database %q: %w", backupID, name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "backup %q of database %q deleted\n", backupID, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func backupsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups delete <database> <backup-id> [flags]

Permanently deletes one archived backup attempt: its stored object and
its history row. Does not touch the rest of the database's backup
history or its recurring schedule. Cannot be undone. Refused (409) while
the named backup is still running.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
