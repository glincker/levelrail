package main

import (
	"context"
	"fmt"
	"io"
)

// runAppVolumeBackupsTrigger implements "app-volume-backups trigger
// <app> <volume> --target ID": POST /api/v1/apps/{name}/volumes/{volume}/backups
// (internal/api/app_volume_backups.go's own handleTriggerVolumeBackup),
// mirroring runBackupsTrigger's exact shape (backups_trigger.go) for the
// volume resource kind. Asynchronous: returns as soon as the attempt is
// recorded and under way, not once the archive and upload actually
// finish; use "app-volume-backups list <app> <volume>" to see whether it
// did.
func runAppVolumeBackupsTrigger(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "app-volume-backups trigger", "print the started backup attempt as JSON to stdout and nothing else", stderr)
	var targetID string
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (required; see the backup target the control plane's own UI or store already has configured)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsTriggerUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups trigger", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	if targetID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--target is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.TriggerVolumeBackup(context.Background(), name, volume, targetID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("trigger backup for %s/%s: %w", name, volume, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup %q for %s/%s started; check \"%s app-volume-backups list %s %s\" for status\n", started.ID, name, volume, prog, name, volume)
	return exitOK
}

func appVolumeBackupsTriggerUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups trigger <app> <volume> --target ID [flags]

Starts a manual backup of an app's named volume and returns as soon as
the attempt is recorded and under way (asynchronous; the real archive
and upload have not necessarily finished by the time this returns).
Check "%[1]s app-volume-backups list <app> <volume>" to see whether it
succeeded.

Flags:
  --target string          backup target id to back up to (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started backup attempt as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
