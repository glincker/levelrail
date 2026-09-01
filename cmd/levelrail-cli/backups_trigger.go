package main

import (
	"context"
	"flag"
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
	fs := flag.NewFlagSet(prog+" backups trigger", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, targetID string
	var jsonOut bool
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (required; see the backup target the control plane's own UI or store already has configured)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	var profileFlag string
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print the started backup attempt as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsTriggerUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
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

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))

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
