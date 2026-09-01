package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppVolumeBackupsList implements "app-volume-backups list <app>
// <volume>": GET /api/v1/apps/{name}/volumes/{volume}/backups
// (internal/api/app_volume_backups.go's own handleListVolumeBackupHistory),
// mirroring runBackupsList's exact shape (backups_list.go) for the
// volume resource kind.
func runAppVolumeBackupsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "app-volume-backups list", "print backup history as a JSON array to stdout and nothing else", stderr)
	var limitFlag int
	var beforeFlag string
	fs.IntVar(&limitFlag, "limit", 0, "max attempts to return (default: server default)")
	fs.StringVar(&beforeFlag, "before", "", "only show attempts started before this RFC3339 timestamp (page backward using the STARTED column of a prior run)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups list", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	history, err := client.ListVolumeBackups(context.Background(), name, volume, apiclient.ListBackupsOptions{
		Limit:  limitFlag,
		Before: beforeFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list backups for %s/%s: %w", name, volume, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, history); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printBackupHistoryTable(stdout, history)
	return exitOK
}

func appVolumeBackupsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups list <app> <volume> [flags]

Lists an app's named volume's backup attempt history.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print backup history as a JSON array to stdout, nothing else
  --limit int              max attempts to return (default: server default)
  --before string          only show attempts started before this RFC3339 timestamp
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
