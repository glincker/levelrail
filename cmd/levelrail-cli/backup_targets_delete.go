package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupTargetsDelete implements "backup-targets delete <id>": DELETE
// /api/v1/backup-targets/{id}. Blocked (409) server-side while any
// backup_history row still references this target, the same foreign-key
// guard DeleteBackupTarget's own doc comment describes.
func runBackupTargetsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "backup-targets delete", "backup target id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteBackupTarget(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete backup target %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "backup target %q disconnected\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func backupTargetsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets delete <id> [flags]

Disconnects a backup target. Fails with a conflict if any database's
backup history still references it.

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
