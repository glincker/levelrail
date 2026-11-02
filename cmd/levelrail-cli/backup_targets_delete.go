package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Returns 409 while a live schedule, policy or running job uses this target.
func runBackupTargetsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	yes := fs.Bool("yes", false, "confirm deleting a target that still has backup history")
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

	if !*yes {
		n, herr := countTargetHistory(context.Background(), client, id)
		if herr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("check backup history of %q: %w", id, herr))
		}
		if n > 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("backup target %q still has backup history; deleting it keeps the objects in the bucket but they can no longer be restored from the console. Re-run with --yes to delete it", id))
		}
	}

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

// countTargetHistory returns 1 as soon as any recorded backup used the target, paging by start time.
func countTargetHistory(ctx context.Context, client *Client, id string) (int, error) {
	before := ""
	for {
		page, err := client.ListAllBackups(ctx, apiclient.ListBackupsOptions{Before: before})
		if err != nil {
			return 0, err
		}
		if len(page) == 0 {
			return 0, nil
		}
		for _, b := range page {
			if b.TargetID == id {
				return 1, nil
			}
		}
		next := page[len(page)-1].StartedAt
		if next == "" || next == before {
			return 0, nil
		}
		before = next
	}
}

func backupTargetsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets delete <id> [flags]

Disconnects a backup target. Fails with a conflict, listing what uses it, while
a live schedule, policy, running backup or restore, or the backup history of an
existing database or app still references it. History of deleted resources is
kept and shown as target deleted, but those backups cannot be restored from the
console. The objects stay in the bucket, and creating a target again does not
reattach the history.

Flags:
  --yes                    confirm deleting a target that still has backup history
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
