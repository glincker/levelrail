package main

import (
	"context"
	"fmt"
	"io"
)

// runPITRRestore implements "pitr restore <database> --base-backup ID
// --target-time RFC3339 [--confirm NAME]": POST
// /api/v1/databases/{name}/pitr-restore (internal/api/pitr.go's own
// handleTriggerPITRRestore), AbilityRoot-gated server-side, the same
// tier "backups restore" uses and for the identical reason
// (backups_restore.go's own doc comment): this overwrites a live
// database's real data in place, with no undo beyond restoring again.
// resolveRestoreConfirmation (backups_restore.go) is reused as-is, the
// same "type the database's exact name" gate that command already
// establishes: databaseName is the only thing being confirmed, the
// destructive-restore shape doesn't change just because the source is a
// timestamp instead of a specific backup ID.
func runPITRRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr restore", "print the started restore attempt as JSON to stdout and nothing else", stderr)
	var baseBackupID, targetTime, confirm string
	fs.StringVar(&baseBackupID, "base-backup", "", "id of a previously succeeded base backup to replay WAL forward from (required)")
	fs.StringVar(&targetTime, "target-time", "", "RFC3339 timestamp to restore to (required; must fall within \"pitr status\"'s reported window)")
	fs.StringVar(&confirm, "confirm", "", "must exactly equal the database name to skip the interactive confirmation prompt")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pitrRestoreUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "pitr restore", "database name")
	if !ok {
		return exitUsage
	}

	if baseBackupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--base-backup is required"))
	}
	if targetTime == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target-time is required"))
	}

	if err := resolveRestoreConfirmation(name, confirm, stdin, stderr); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.TriggerPITRRestore(context.Background(), name, baseBackupID, targetTime)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore database %q to %q: %w", name, targetTime, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stdout, "point-in-time restore %q of database %q to %q started; check \"%s pitr status %s\" for the database's own condition once it finishes\n", started.ID, name, targetTime, prog, name)
	})
}

func pitrRestoreUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr restore <database> --base-backup ID --target-time RFC3339 [--confirm NAME] [flags]

Overwrites a live database's data in place, restored to the exact
timestamp given by --target-time, replaying archived WAL forward from
--base-backup. Destructive and not undoable short of restoring again.
--target-time must fall within the window "%[1]s pitr status <database>"
reports; a timestamp outside it is rejected before anything is touched.

Requires typing the database's exact name to confirm: pass it as
--confirm, or leave --confirm off to be prompted for it interactively.
A missing or mismatched confirmation refuses to call the API at all.

Flags:
  --base-backup string     id of a previously succeeded base backup to replay from (required)
  --target-time string     RFC3339 timestamp to restore to (required)
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
