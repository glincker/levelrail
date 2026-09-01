package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runBackupTargetsList implements "backup-targets list": GET
// /api/v1/backup-targets.
func runBackupTargetsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "backup-targets list", "print backup targets as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	targets, err := client.ListBackupTargets(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list backup targets: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, targets); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printBackupTargetsTable(stdout, targets)
	return exitOK
}

func printBackupTargetsTable(out io.Writer, targets []backupTargetResource) {
	if len(targets) == 0 {
		_, _ = fmt.Fprintln(out, "no backup targets")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tPROVIDER\tBUCKET\tREGION\tCREATED")
	for _, t := range targets {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Provider, t.Bucket, t.Region, t.CreatedAt)
	}
	_ = tw.Flush()
}

func backupTargetsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets list [flags]

Lists every connected backup target. Credentials are never included.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print backup targets as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
