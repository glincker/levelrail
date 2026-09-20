package main

import (
	"context"
	"fmt"
	"io"
)

// runAppVolumeBackupsDownload implements
// "app-volume-backups download <app> <volume> <backup-id>": GET
// /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download, the
// app service volume counterpart of runBackupsDownload
// (backups_download.go). See that function's own doc comment for the
// raw-bytes-to-stdout convention this mirrors.
func runAppVolumeBackupsDownload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "app-volume-backups download", "not applicable, the response body is always the raw backup file", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsDownloadUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 3 {
		_, _ = fmt.Fprintf(stderr, "%s: app-volume-backups download requires an app name, a volume name, and a backup id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, volume, backupID := rest[0], rest[1], rest[2]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	data, err := client.DownloadVolumeBackup(context.Background(), name, volume, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("download backup %q of %s/%s: %w", backupID, name, volume, err))
	}
	_, _ = stdout.Write(data)
	return exitOK
}

func appVolumeBackupsDownloadUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups download <app> <volume> <backup-id> [flags]

Streams one succeeded backup's own object (a volume archive) straight to
stdout. Only a backup with status "succeeded" can be downloaded.

Output goes to stdout only (errors and usage go to stderr), so redirect
it to save a copy: %[1]s app-volume-backups download web data bkh_1 > data.tar

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
