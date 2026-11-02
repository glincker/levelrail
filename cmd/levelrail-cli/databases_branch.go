package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDatabasesBranch dispatches "databases branch create".
func runDatabasesBranch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesBranchUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesBranchUsage(prog))
		return exitOK
	case "create":
		return runDatabasesBranchCreate(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases branch subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesBranchUsage(prog))
		return exitUsage
	}
}

func databasesBranchUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases branch create <source> <new-name> (--from-backup ID | --at TIME) [flags]

Creates a new, independent database from a backup of <source>, or from a
point in time when point-in-time recovery is enabled on it (MySQL and
MariaDB). <source> is never changed. The branch gets its own generated
credentials, no linked apps and no pooler.

Flags:
  --from-backup string  id of a succeeded backup of <source>
  --at string           RFC3339 time to branch from
  --project string      project id for the branch (default: the source's project)
`, prog)
}

func runDatabasesBranchCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd := "databases branch create"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the started branch as JSON to stdout and nothing else", stderr)
	var backupID, at, projectID string
	fs.StringVar(&backupID, "from-backup", "", "id of a succeeded backup of the source database")
	fs.StringVar(&at, "at", "", "RFC3339 time to branch from (needs point-in-time recovery on the source)")
	fs.StringVar(&projectID, "project", "", "project id for the branch")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesBranchUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires a source database and a new name\n\n", prog, cmd)
		fs.Usage()
		return exitUsage
	}
	if (backupID == "") == (at == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("give exactly one of --from-backup or --at"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.CreateDatabaseBranch(context.Background(), rest[0], apiclient.CreateDatabaseBranchRequest{
		Name: rest[1], BackupID: backupID, At: at, ProjectID: projectID,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("branch database %q as %q: %w", rest[0], rest[1], err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "branch %q of database %q started; check \"%s databases get %s\" for status\n", res.Name, res.BranchedFrom, prog, res.Name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
