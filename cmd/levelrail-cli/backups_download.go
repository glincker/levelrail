package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupsDownload implements "backups download <database> <backup-id>":
// GET /api/v1/databases/{name}/backups/{historyId}/download
// (internal/api/backup_download.go's own handleDownloadBackup), writing
// the backup's raw object bytes straight to stdout, the same
// "no --json/--output/--query, redirect to save a copy" convention
// "apps deploys logs" already establishes for its own raw-bytes download
// (apps_deploys.go's runAppsDeploysLogs).
func runBackupsDownload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups download", "not applicable, the response body is always the raw backup file", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsDownloadUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "backups download", "a database name and a backup id", 2)
	if !ok {
		return exitUsage
	}
	name, backupID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	data, err := client.DownloadBackup(context.Background(), name, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("download backup %q of database %q: %w", backupID, name, err))
	}
	_, _ = stdout.Write(data)
	return exitOK
}

func backupsDownloadUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups download <database> <backup-id> [flags]

Streams one succeeded backup's own object (a database dump) straight to
stdout. Only a backup with status "succeeded" can be downloaded.

Output goes to stdout only (errors and usage go to stderr), so redirect
it to save a copy: %[1]s backups download mydb bkh_1 > mydb.dump

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
