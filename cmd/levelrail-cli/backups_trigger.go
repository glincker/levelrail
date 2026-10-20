package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Asynchronous: returns once the attempt is recorded, so check backups list for the outcome.
func runBackupsTrigger(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups trigger", "print the started backup attempt as JSON to stdout and nothing else", stderr)
	var targetID string
	fs.StringVar(&targetID, "target", "", "backup target id to back up to (default: the only configured target)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsTriggerUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups trigger requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if targetID == "" {
		id, err := soleBackupTarget(context.Background(), client, prog)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		targetID = id
	}

	started, err := client.TriggerBackup(context.Background(), name, targetID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("trigger backup for database %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, started, func() {
		_, _ = fmt.Fprintf(stdout, "backup %q for database %q started; check \"%s backups list %s\" for status\n", started.ID, name, prog, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// soleBackupTarget returns the only configured target's id, or says how to choose when there are none or several.
func soleBackupTarget(ctx context.Context, client *Client, prog string) (string, error) {
	targets, err := client.ListBackupTargets(ctx)
	if err != nil {
		return "", fmt.Errorf("list backup targets: %w", err)
	}
	switch len(targets) {
	case 1:
		return targets[0].ID, nil
	case 0:
		return "", newValidationError("no backup target is configured yet; create one with \"%s backup-targets create -h\" or in the dashboard under Backups, then retry", prog)
	}
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.ID+" ("+t.Name+")")
	}
	return "", newValidationError("%d backup targets exist, pick one with --target: %s", len(targets), strings.Join(ids, ", "))
}

func backupsTriggerUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups trigger <database> [--target ID] [flags]

Starts a manual backup of an existing database and returns as soon as
the attempt is recorded and under way (asynchronous; the real dump and
upload have not necessarily finished by the time this returns). Check
"%[1]s backups list <database>" to see whether it succeeded.

Flags:
  --target string          backup target id to back up to (default: the only configured target)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the started backup attempt as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
