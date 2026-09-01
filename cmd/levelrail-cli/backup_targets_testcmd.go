package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupTargetsTest implements "backup-targets test <id>": POST
// /api/v1/backup-targets/{id}/test, probing the target's stored
// credentials against its configured bucket without uploading or
// deleting anything.
func runBackupTargetsTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets test", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsTestUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "backup-targets test", "backup target id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.TestBackupTarget(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test backup target %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"ok": true}, func() {
		_, _ = fmt.Fprintf(stdout, "backup target %q connected successfully\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func backupTargetsTestUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets test <id> [flags]

Probes a connected backup target's bucket over its stored credentials
with a lightweight HeadBucket call, catching a bad or stale credential or
a renamed/deleted bucket before the next scheduled backup fails against
it silently. Never uploads or deletes anything.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"ok": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
