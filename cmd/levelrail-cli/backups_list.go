package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runBackupsList implements "backups list <database>": GET
// /api/v1/databases/{name}/backups (internal/api/backups.go's own
// handleListBackupHistory), AbilityRead-gated: passive visibility into
// attempts already made.
func runBackupsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "backups list", "print backup history as a JSON array to stdout and nothing else", stderr)
	var beforeFlag string
	var limitFlag int
	fs.IntVar(&limitFlag, "limit", 0, "max attempts to return (default: server default)")
	fs.StringVar(&beforeFlag, "before", "", "only show attempts started before this RFC3339 timestamp (page backward using the STARTED column of a prior run)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsListUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: backups list requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	history, err := client.ListBackups(context.Background(), name, apiclient.ListBackupsOptions{
		Limit:  limitFlag,
		Before: beforeFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list backups for database %q: %w", name, err))
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

// printBackupHistoryTable prints a compact, aligned table of backup
// attempts, the same shape output.go's own list-command helpers already
// establish.
func printBackupHistoryTable(out io.Writer, history []backupHistoryResource) {
	if len(history) == 0 {
		_, _ = fmt.Fprintln(out, "no backups")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTARGET\tSTATUS\tSIZE\tSTARTED\tFINISHED")
	for _, h := range history {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", h.ID, h.TargetID, h.Status, h.SizeBytes, h.StartedAt, h.FinishedAt)
	}
	_ = tw.Flush()
}

func backupsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups list <database> [flags]

Lists a database's backup attempt history.

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
