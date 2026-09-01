package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppVolumeBackupsVerify implements "app-volume-backups verify <app>
// <volume> --backup ID":
// POST /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verify
// (internal/api/app_volume_backup_verify.go's own handleVerifyVolumeBackup),
// mirroring runBackupsVerify's exact shape (backups_verify.go) for the
// volume resource kind. Never attempts a live restore.
func runAppVolumeBackupsVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "app-volume-backups verify", "print the started verification attempt as JSON to stdout and nothing else", stderr)
	var backupID string
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to verify (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsVerifyUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups verify", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.VerifyVolumeBackup(context.Background(), name, volume, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("verify backup %q for %s/%s: %w", backupID, name, volume, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "verification %q of backup %q started; check \"%s app-volume-backups verifications %s %s --backup %s\" for status\n", started.ID, backupID, prog, name, volume, backupID)
	return exitOK
}

func appVolumeBackupsVerifyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups verify <app> <volume> --backup ID [flags]

Re-downloads a previously succeeded backup and confirms it is still
intact (checksum, size, and a non-empty check), without ever attempting a
live restore. Returns as soon as the attempt is recorded and under way.
Check "%[1]s app-volume-backups verifications <app> <volume> --backup ID"
to see whether it passed.

Flags:
  --backup string          id of a previously succeeded backup to verify (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started verification attempt as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
