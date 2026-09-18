package main

import (
	"context"
	"fmt"
	"io"
)

// runAppVolumeBackupsDelete implements
// "app-volume-backups delete <app> <volume> <backup-id>": DELETE
// /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}, the app
// service volume counterpart of runBackupsDelete (backups_delete.go). See
// that function's own doc comment for why this needs no separate
// --confirm flag beyond the backup id itself.
func runAppVolumeBackupsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "app-volume-backups delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 3 {
		_, _ = fmt.Fprintf(stderr, "%s: app-volume-backups delete requires an app name, a volume name, and a backup id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, volume, backupID := rest[0], rest[1], rest[2]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteVolumeBackup(context.Background(), name, volume, backupID); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete backup %q of %s/%s: %w", backupID, name, volume, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "backup %q of %s/%s deleted\n", backupID, name, volume)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appVolumeBackupsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups delete <app> <volume> <backup-id> [flags]

Permanently deletes one archived backup attempt: its stored object and
its history row. Does not touch the rest of the volume's backup history
or its recurring schedule. Cannot be undone. Refused (409) while the
named backup is still running.

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
