package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// resolveRestoreConfirmation is runBackupsRestore's confirmation gate,
// factored out as a pure-ish function (its only I/O is the prompt/read
// pair, both taken as parameters) so it's directly table-driven
// testable without a real terminal or a fake HTTP server: exactly what
// the task's own requirement asks for ("a missing/mismatched --confirm
// value refuses to call the API at all").
//
// Mirrors the web frontend's own destructive-confirmation pattern
// (web/src/components/RestoreBackupDialog.tsx: "Type ‘{databaseName}’ to
// confirm", the confirm action stays disabled until the typed text
// matches exactly): confirmFlag given must equal databaseName exactly,
// case-sensitively, no trimming beyond what the caller's shell already
// did to its own argument; confirmFlag omitted falls back to the same
// exact-match check against one line read interactively from stdin, so
// a script that doesn't pass --confirm at all is asked, not silently
// allowed through, and a human runs the exact same "type the name"
// motion the web UI already trained them on.
func resolveRestoreConfirmation(databaseName, confirmFlag string, stdin io.Reader, stderr io.Writer) error {
	if confirmFlag != "" {
		if confirmFlag != databaseName {
			return newValidationError("--confirm %q does not match database name %q, refusing to restore", confirmFlag, databaseName)
		}
		return nil
	}

	_, _ = fmt.Fprintf(stderr, "This overwrites database %q's live data in place. This cannot be undone short of restoring again from a different backup.\n", databaseName)
	_, _ = fmt.Fprintf(stderr, "Type %q to confirm: ", databaseName)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		// No confirmation was ever obtained (EOF on stdin, e.g. a
		// script that ran this without --confirm and with no
		// terminal attached): the same "refuse, don't hang, don't
		// guess" outcome a mismatched --confirm gets, classified as a
		// validationError for the same reason, not a network failure.
		return newValidationError("read confirmation: %v", err)
	}
	typed := strings.TrimSpace(line)
	if typed != databaseName {
		return newValidationError("confirmation %q does not match database name %q, refusing to restore", typed, databaseName)
	}
	return nil
}

// runBackupsRestore implements "backups restore <database> --backup ID
// --confirm NAME": POST /api/v1/databases/{name}/restore
// (internal/api/restore.go's own handleTriggerRestore), AbilityRoot-
// gated server-side, the same tier node management and placement use,
// because this is the single most destructive endpoint in the whole
// API: it overwrites a live database's real data in place, with no undo
// beyond restoring again from a different backup (handleTriggerRestore's
// own doc comment). resolveRestoreConfirmation above is checked, and can
// refuse, before this ever builds a Client or reaches the network: a
// caller who gets the confirmation wrong never sends a byte to the
// server.
func runBackupsRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups restore", "print the started restore attempt as JSON to stdout and nothing else", stderr)
	var backupID, confirm string
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to restore from (required)")
	fs.StringVar(&confirm, "confirm", "", "must exactly equal the database name to skip the interactive confirmation prompt")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsRestoreUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups restore requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	if err := resolveRestoreConfirmation(name, confirm, stdin, stderr); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.TriggerRestore(context.Background(), name, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore database %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, started, func() {
		_, _ = fmt.Fprintf(stdout, "restore %q of database %q from backup %q started; check \"%s backups list %s\" for status\n", started.ID, name, backupID, prog, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func backupsRestoreUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups restore <database> --backup ID [--confirm NAME] [flags]

Overwrites a live database's data in place from a previously succeeded
backup attempt. Destructive and not undoable short of restoring again
from a different backup. Requires typing the database's exact name to
confirm: pass it as --confirm, or leave --confirm off to be prompted for
it interactively (same "type the name" pattern the web UI's own restore
dialog uses). A missing or mismatched confirmation refuses to call the
API at all.

Flags:
  --backup string          id of a previously succeeded backup to restore from (required)
  --confirm string        must exactly equal <database> to skip the interactive prompt
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started restore attempt as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
