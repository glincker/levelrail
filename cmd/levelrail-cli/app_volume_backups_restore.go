package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

// resolveVolumeRestoreConfirmation mirrors resolveRestoreConfirmation's
// exact shape (backups_restore.go), the app volume counterpart: the
// value to type is "app/volume", the same composite identifier this
// codebase's own log labels already use for a service volume
// (internal/backup.Scheduler's own volumeScheduleKey), since a volume
// alone has no single global name to type the way a database does.
func resolveVolumeRestoreConfirmation(appName, volumeName, confirmFlag string, stdin io.Reader, stderr io.Writer) error {
	target := appName + "/" + volumeName

	if confirmFlag != "" {
		if confirmFlag != target {
			return newValidationError("--confirm %q does not match %q, refusing to restore", confirmFlag, target)
		}
		return nil
	}

	_, _ = fmt.Fprintf(stderr, "This overwrites %q's live volume contents in place. This cannot be undone short of restoring again from a different backup.\n", target)
	_, _ = fmt.Fprintf(stderr, "Type %q to confirm: ", target)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return newValidationError("read confirmation: %v", err)
	}
	typed := strings.TrimSpace(line)
	if typed != target {
		return newValidationError("confirmation %q does not match %q, refusing to restore", typed, target)
	}
	return nil
}

// runAppVolumeBackupsRestore implements "app-volume-backups restore
// <app> <volume> --backup ID --confirm APP/VOLUME":
// POST /api/v1/apps/{name}/volumes/{volume}/restore
// (internal/api/app_volume_restore.go's own handleTriggerVolumeRestore),
// AbilityRoot-gated server-side, the same tier the database restore
// route uses (see that handler's own doc comment): this overwrites a
// volume's real data in place, with no undo beyond restoring again from
// a different backup. resolveVolumeRestoreConfirmation above is checked,
// and can refuse, before this ever builds a Client or reaches the
// network.
func runAppVolumeBackupsRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "app-volume-backups restore", "print the started restore attempt as JSON to stdout and nothing else", stderr)
	var backupID, confirm string
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to restore from (required)")
	fs.StringVar(&confirm, "confirm", "", "must exactly equal \"<app>/<volume>\" to skip the interactive confirmation prompt")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsRestoreUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest, ok := requireArgs(fs, stderr, prog, "app-volume-backups restore", "an app name and a volume name", 2)
	if !ok {
		return exitUsage
	}
	name, volume := rest[0], rest[1]

	if backupID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--backup is required"))
	}

	if err := resolveVolumeRestoreConfirmation(name, volume, confirm, stdin, stderr); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.TriggerVolumeRestore(context.Background(), name, volume, backupID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore %s/%s: %w", name, volume, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "restore %q of %s/%s from backup %q started; check \"%s app-volume-backups list %s %s\" for status\n", started.ID, name, volume, backupID, prog, name, volume)
	return exitOK
}

func appVolumeBackupsRestoreUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups restore <app> <volume> --backup ID [--confirm APP/VOLUME] [flags]

Overwrites an app's named volume's live contents in place from a
previously succeeded backup attempt. Destructive and not undoable short
of restoring again from a different backup. Requires typing "<app>/<volume>"
exactly to confirm: pass it as --confirm, or leave --confirm off to be
prompted for it interactively. A missing or mismatched confirmation
refuses to call the API at all.

Flags:
  --backup string          id of a previously succeeded backup to restore from (required)
  --confirm string        must exactly equal "<app>/<volume>" to skip the interactive prompt
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started restore attempt as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
