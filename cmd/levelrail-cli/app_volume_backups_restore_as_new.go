package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppVolumeBackupsRestoreAsNew implements "app-volume-backups
// restore-as-new <app> <volume> --backup ID [--new-volume-name NAME]":
// POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new
// (internal/api/app_volume_clone_restore.go's own handleVolumeCloneRestore),
// write:sensitive-gated server-side, not root: the app service volume
// counterpart of runBackupsRestoreAsNew. Unlike "app-volume-backups
// restore" this never touches <app>/<volume>'s own live contents, it only
// creates a brand-new, standalone Docker volume and restores the named
// backup into it. No --confirm gate, the same reasoning
// runBackupsRestoreAsNew's own doc comment gives: there is nothing here
// for a misclick to destroy.
func runAppVolumeBackupsRestoreAsNew(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" app-volume-backups restore-as-new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, backupID, newVolumeName string
	var jsonOut bool
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to restore from (required)")
	fs.StringVar(&newVolumeName, "new-volume-name", "", "name for the brand-new, standalone Docker volume (default: a generated name, see the started attempt's own new_volume_name)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	var profileFlag string
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print the started clone-restore attempt as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsRestoreAsNewUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups restore-as-new", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))

	started, err := client.TriggerVolumeCloneRestore(context.Background(), name, volume, triggerVolumeCloneRestoreRequest{
		BackupID:      backupID,
		NewVolumeName: newVolumeName,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore %s/%s as new volume: %w", name, volume, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "clone-restore %q of %s/%s from backup %q into new volume %q started\n", started.ID, name, volume, backupID, started.NewVolumeName)
	return exitOK
}

func appVolumeBackupsRestoreAsNewUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups restore-as-new <app> <volume> --backup ID [--new-volume-name NAME] [flags]

Creates a brand-new, standalone Docker volume (not attached to any app)
and restores a previously succeeded backup of <app>'s <volume> into it.
Never touches <app>/<volume>'s own live contents: the safe alternative to
"app-volume-backups restore" for inspecting a backup's contents without
risking the original.

Flags:
  --backup string             id of a previously succeeded backup to restore from (required)
  --new-volume-name string   name for the new Docker volume (default: a generated name)
  --token string               API token (default: %[2]s env var, then the credentials file)
  --api-url string            control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string            named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                        print the started clone-restore attempt as JSON to stdout, nothing else
  -h, --help                  show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
