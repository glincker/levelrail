package main

import (
	"context"
	"fmt"
	"io"
)

// runAppVolumeBackupsVerifications implements "app-volume-backups
// verifications <app> <volume> --backup ID":
// GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verifications
// (internal/api/app_volume_backup_verify.go's own
// handleListVolumeBackupVerifications), mirroring
// runBackupsVerifications's exact shape (backups_verifications.go) for
// the volume resource kind. Reuses printBackupVerificationsTable
// (backups_verifications.go) as-is: the table shape is identical.
func runAppVolumeBackupsVerifications(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "app-volume-backups verifications", "print verification history as a JSON array to stdout and nothing else", stderr)
	var backupID string
	fs.StringVar(&backupID, "backup", "", "id of a backup to show verification history for (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsVerificationsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups verifications", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	verifications, err := client.ListVolumeBackupVerifications(context.Background(), name, volume, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list verifications for backup %q: %w", backupID, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, verifications, func() { printBackupVerificationsTable(stdout, verifications) })
}

func appVolumeBackupsVerificationsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups verifications <app> <volume> --backup ID [flags]

Lists a backup's verification attempt history, newest first.

Flags:
  --backup string          id of a backup to show verification history for (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print verification history as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
