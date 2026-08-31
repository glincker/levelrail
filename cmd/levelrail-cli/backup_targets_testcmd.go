package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runBackupTargetsTest implements "backup-targets test <id>": POST
// /api/v1/backup-targets/{id}/test, probing the target's stored
// credentials against its configured bucket without uploading or
// deleting anything.
func runBackupTargetsTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "backup-targets test", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsTestUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "backup-targets test", "backup target id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	if err := client.TestBackupTarget(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test backup target %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"ok": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup target %q connected successfully\n", id)
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
  --json                    print {"ok": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
