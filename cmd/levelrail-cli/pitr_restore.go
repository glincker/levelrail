package main

import (
	"context"
	"fmt"
	"io"
)

// Postgres restores overwrite live data in place, so they are root-gated and confirmed by typing the database name.
// MySQL and MariaDB restores always create a new database (--restore-as), so they need no confirmation.
func runPITRRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr restore", "print the started restore attempt as JSON to stdout and nothing else", stderr)
	var baseBackupID, targetTime, confirm, restoreAs string
	fs.StringVar(&baseBackupID, "base-backup", "", "id of a previously succeeded base backup to replay WAL forward from (required)")
	fs.StringVar(&targetTime, "target-time", "", "RFC3339 timestamp to restore to (required; must fall within \"pitr status\"'s reported window)")
	fs.StringVar(&restoreAs, "restore-as", "", "name of the new database to restore into (required for MySQL and MariaDB, which never overwrite the source)")
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

	if restoreAs == "" {
		if err := resolveRestoreConfirmation(name, confirm, stdin, stderr); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.TriggerPITRRestore(context.Background(), name, baseBackupID, targetTime, restoreAs)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore database %q to %q: %w", name, targetTime, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stdout, "point-in-time restore %q of database %q to %q started; check \"%s pitr status %s\" for the database's own condition once it finishes\n", started.ID, name, targetTime, prog, name)
	})
}

func pitrRestoreUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr restore <database> --base-backup ID --target-time RFC3339 [--confirm NAME | --restore-as NEW] [flags]

Postgres: overwrites a live database's data in place, restored to the exact
timestamp given by --target-time, replaying archived WAL forward from
--base-backup. Destructive and not undoable short of restoring again.

MySQL and MariaDB: pass --restore-as to create a new database from
--base-backup plus the archived binlog up to --target-time. The source is
never touched, so no confirmation is asked.
--target-time must fall within the window "%[1]s pitr status <database>"
reports; a timestamp outside it is rejected before anything is touched.

Requires typing the database's exact name to confirm: pass it as
--confirm, or leave --confirm off to be prompted for it interactively.
A missing or mismatched confirmation refuses to call the API at all.

Flags:
  --base-backup string     id of a previously succeeded base backup to replay from (required)
  --target-time string     RFC3339 timestamp to restore to (required)
  --restore-as string      new database to restore into (MySQL and MariaDB)
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
