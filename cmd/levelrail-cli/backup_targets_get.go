package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runBackupTargetsGet implements "backup-targets get <id>": GET
// /api/v1/backup-targets/{id}.
func runBackupTargetsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "backup-targets get", "print the backup target as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsGetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "backup-targets get", "backup target id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	target, err := client.GetBackupTarget(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get backup target %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, target); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printBackupTargetHuman(stdout, target)
	return exitOK
}

func printBackupTargetHuman(out io.Writer, t backupTargetResource) {
	_, _ = fmt.Fprintf(out, "id:        %s\n", t.ID)
	_, _ = fmt.Fprintf(out, "name:      %s\n", t.Name)
	_, _ = fmt.Fprintf(out, "provider:  %s\n", t.Provider)
	if t.Endpoint != "" {
		_, _ = fmt.Fprintf(out, "endpoint:  %s\n", t.Endpoint)
	}
	if t.Region != "" {
		_, _ = fmt.Fprintf(out, "region:    %s\n", t.Region)
	}
	_, _ = fmt.Fprintf(out, "bucket:    %s\n", t.Bucket)
	_, _ = fmt.Fprintf(out, "created:   %s\n", t.CreatedAt)
}

func backupTargetsGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets get <id> [flags]

Shows one backup target. Credentials are never included.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the backup target as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
