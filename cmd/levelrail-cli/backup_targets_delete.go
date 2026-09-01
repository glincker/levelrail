package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runBackupTargetsDelete implements "backup-targets delete <id>": DELETE
// /api/v1/backup-targets/{id}. Blocked (409) server-side while any
// backup_history row still references this target, the same foreign-key
// guard DeleteBackupTarget's own doc comment describes.
func runBackupTargetsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "backup-targets delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "backup-targets delete", "backup target id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteBackupTarget(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete backup target %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup target %q disconnected\n", id)
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
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
