package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// The typed value is "app/volume", since a volume has no global name the way a database does.
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

// This overwrites live volume data in place, so the confirmation check runs before any network call.
func runAppVolumeBackupsRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "app-volume-backups restore", "print the started restore attempt as JSON to stdout and nothing else", stderr)
	var backupID, confirm string
	fs.StringVar(&backupID, "backup", "", "id of a previously succeeded backup to restore from (required)")
	fs.StringVar(&confirm, "confirm", "", "must exactly equal \"<app>/<volume>\" to skip the interactive confirmation prompt")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appVolumeBackupsRestoreUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

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

	if err := renderResult(stdout, of.Format, of.Query, started, func() {
		_, _ = fmt.Fprintf(stdout, "restore %q of %s/%s from backup %q started; check \"%s app-volume-backups list %s %s\" for status\n", started.ID, name, volume, backupID, prog, name, volume)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
