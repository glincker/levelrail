package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupsClone implements "backups clone <database> --new-name NAME
// [--target ID]": POST /api/v1/databases/{name}/clone
// (internal/api/database_clone_now.go's own handleCloneDatabaseNow),
// write:sensitive-gated like "backups restore-as-new". The difference
// from restore-as-new: this takes a fresh backup as part of the same
// call instead of requiring --backup to name one that already
// succeeded, so a database with zero backup history yet can still be
// cloned in one step.
func runBackupsClone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups clone", "print the started clone as JSON to stdout and nothing else", stderr)
	var newName, targetID string
	fs.StringVar(&newName, "new-name", "", "name for the brand-new database this clone creates (required)")
	fs.StringVar(&targetID, "target", "", "backup target id to clone through (default: the source database's own configured backup target)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsCloneUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups clone requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if newName == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--new-name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.CloneDatabaseNow(context.Background(), name, cloneNowRequest{
		NewName:  newName,
		TargetID: targetID,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clone database %q into %q: %w", name, newName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, started, func() {
		_, _ = fmt.Fprintf(stdout, "cloning %q into new database %q started (backup %s, clone-restore %s); check \"%s databases get %s\" for status\n", name, newName, started.BackupHistoryID, started.CloneRestoreID, prog, newName)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func backupsCloneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups clone <database> --new-name NAME [flags]

Takes a fresh backup of <database> and restores it into a brand-new
database, in one step: unlike "backups restore-as-new", no existing
successful backup is required first. Never touches <database>'s own
live data.

Flags:
  --new-name string        name for the brand-new database (required)
  --target string          backup target id to clone through (default: the source database's own configured backup target)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started clone as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
