package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupsTrigger implements "backups trigger <database> --target
// ID": POST /api/v1/databases/{name}/backups (internal/api/backups.go's
// own handleTriggerBackup), AbilityWriteSensitive-gated. Asynchronous,
// same as the server-side handler's own doc comment: this returns as
// soon as the attempt is recorded and under way (202 Accepted
// server-side), not once the real dump and upload actually finish; use
// "backups list <database>" to see whether it did.
func runBackupsTrigger(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "backups trigger", "print the started backup attempt as JSON to stdout and nothing else", stderr)
	var targetID string
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (required; see the backup target the control plane's own UI or store already has configured)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsTriggerUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups trigger requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if targetID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.TriggerBackup(context.Background(), name, targetID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("trigger backup for database %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup %q for database %q started; check \"%s backups list %s\" for status\n", started.ID, name, prog, name)
	return exitOK
}

func backupsTriggerUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups trigger <database> --target ID [flags]

Starts a manual backup of an existing database and returns as soon as
the attempt is recorded and under way (asynchronous; the real dump and
upload have not necessarily finished by the time this returns). Check
"%[1]s backups list <database>" to see whether it succeeded.

Flags:
  --target string          backup target id to back up to (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started backup attempt as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
