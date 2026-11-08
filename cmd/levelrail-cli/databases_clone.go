package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Sugar over restore-as-new: never touches the source database's data.
func runDatabasesClone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases clone", "print the started clone as JSON to stdout and nothing else", stderr)
	var backupID, targetID, version, projectID string
	var wait time.Duration
	fs.StringVar(&backupID, "backup", "", "id of a succeeded backup to clone from (default: take a fresh backup first)")
	fs.StringVar(&targetID, "target", "", "backup target id for the fresh backup (default: the only configured target)")
	fs.StringVar(&version, "version", "", "engine version for the new database (default: the source's version)")
	fs.StringVar(&projectID, "project", "", "project id to assign the new database to")
	fs.DurationVar(&wait, "wait", 0, "how long to wait for the fresh backup (default: "+exportWaitEnv+" env var, then 15m)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesCloneUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases clone requires a source database and a new name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	src, newName := rest[0], rest[1]
	if !validDatabaseName(src) || !validDatabaseName(newName) {
		return reportError(stdout, stderr, jsonOut, newValidationError("database names use letters, digits, '-' and '_' and are at most 63 characters"))
	}
	if wait <= 0 {
		wait = durationFromEnv(lookupEnv, exportWaitEnv, exportWaitFallback)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if backupID == "" {
		id, err := pickExportBackup(ctx, client, prog, src, targetID, false, wait, exportPollInterval(lookupEnv), stderr)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		backupID = id
	}
	started, err := client.TriggerCloneRestore(ctx, src, triggerCloneRestoreRequest{
		BackupID:  backupID,
		NewName:   newName,
		Version:   version,
		ProjectID: projectID,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clone database %q as %q: %w", src, newName, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, started, func() {
		_, _ = fmt.Fprintf(stdout, "clone %q of database %q from backup %q into new database %q started; check \"%s databases get %s\" for status\n", started.ID, src, backupID, newName, prog, newName)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func databasesCloneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases clone <source> <new-name> [--backup ID] [flags]

Creates a brand-new database from a backup of <source>. Without --backup a
fresh backup is taken first and awaited. <source> is never changed.

Flags:
  --backup string    id of a succeeded backup to clone from
  --target string    backup target id for the fresh backup (default: the only configured target)
  --version string   engine version for the new database (default: the source's version)
  --project string   project id to assign the new database to
  --wait duration    how long to wait for the fresh backup (default: %[5]s env var, then 15m)
  --token string     API token (default: %[2]s env var, then the credentials file)
  --api-url string   control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string   named credentials profile to read (overrides APP_PROFILE, default "default")
  --json             print the started clone as JSON to stdout, nothing else
  --output string    output format: json, table, or text
  --query string     JMESPath expression to filter the result before printing
  -h, --help         show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL, exportWaitEnv)
}

func validDatabaseName(n string) bool {
	if n == "" || len(n) > 63 {
		return false
	}
	for _, r := range n {
		alnum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !alnum && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
