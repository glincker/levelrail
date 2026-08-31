package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runBackupsRestoreAsNew implements "backups restore-as-new <database>
// --backup ID --new-name NAME": POST /api/v1/databases/{name}/restore-as-new
// (internal/api/database_clone_restore.go's own handleCloneRestore),
// write:sensitive-gated server-side, not root: unlike "backups restore"
// this never touches <database>'s own live data, it only creates a new
// database and restores the named backup into it, the same sensitivity
// tier "backups trigger" already uses (real work against a real bucket
// with a real stored credential). No --confirm gate the way
// resolveRestoreConfirmation guards the in-place restore: there is
// nothing here for a misclick to destroy, since the worst case is an
// extra database the operator can delete like any other.
func runBackupsRestoreAsNew(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" backups restore-as-new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, backupID, newName, version, projectID string
	var jsonOut bool
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to restore from (required)")
	fs.StringVar(&newName, "new-name", "", "name for the brand-new database this backup is restored into (required)")
	fs.StringVar(&version, "version", "", "engine version for the new database (default: the source database's own current version)")
	fs.StringVar(&projectID, "project", "", "project id to assign the new database to")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print the started clone-restore attempt as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsRestoreAsNewUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups restore-as-new requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}
	if newName == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--new-name is required"))
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	started, err := client.TriggerCloneRestore(context.Background(), name, triggerCloneRestoreRequest{
		BackupID:  backupID,
		NewName:   newName,
		Version:   version,
		ProjectID: projectID,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore database %q as new database %q: %w", name, newName, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "clone-restore %q of database %q from backup %q into new database %q started; check \"%s databases get %s\" for status\n", started.ID, name, backupID, newName, prog, newName)
	return exitOK
}

func backupsRestoreAsNewUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups restore-as-new <database> --backup ID --new-name NAME [flags]

Creates a brand-new database and restores a previously succeeded backup
of <database> into it. Never touches <database>'s own live data: the
safe alternative to "backups restore" for testing a migration against
real data or standing up a staging copy.

Flags:
  --backup string          id of a previously succeeded backup to restore from (required)
  --new-name string        name for the brand-new database (required)
  --version string         engine version for the new database (default: the source database's own current version)
  --project string         project id to assign the new database to
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the started clone-restore attempt as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
