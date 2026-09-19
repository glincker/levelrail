package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runBackupsListAll implements "backups list-all": GET /api/v1/backups
// (internal/api/backups.go's own handleListAllBackups), the instance-wide
// counterpart of "backups list <database>" (backups_list.go) and
// "app-volume-backups list <app> <volume>" (app_volume_backups_list.go).
// Named list-all rather than reusing "list" bare: "backups list" already
// takes a required database-name argument, so overloading it based on
// argument count would make the same subcommand mean two different
// queries depending on how many args a caller happened to pass.
func runBackupsListAll(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups list-all", "print backup history as a JSON array to stdout and nothing else", stderr)
	var limitFlag int
	var beforeFlag string
	fs.IntVar(&limitFlag, "limit", 0, "max attempts to return (default: server default)")
	fs.StringVar(&beforeFlag, "before", "", "only show attempts started before this RFC3339 timestamp (page backward using the STARTED column of a prior run)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsListAllUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if len(fs.Args()) != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: backups list-all takes no positional arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	history, err := client.ListAllBackups(context.Background(), apiclient.ListBackupsOptions{
		Limit:  limitFlag,
		Before: beforeFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list all backups: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, history, func() { printAllBackupHistoryTable(stdout, history) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// printAllBackupHistoryTable is printBackupHistoryTable's instance-wide
// counterpart: an extra RESOURCE column since, unlike a per-resource
// list, the caller doesn't already know which database or app volume
// each row belongs to.
func printAllBackupHistoryTable(out io.Writer, history []backupHistoryResource) {
	if len(history) == 0 {
		_, _ = fmt.Fprintln(out, "no backups")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tRESOURCE\tTARGET\tSTATUS\tSIZE\tSTARTED\tFINISHED")
	for _, h := range history {
		resource := h.DatabaseName
		if h.ResourceKind == "volume" {
			resource = h.ServiceName + "/" + h.VolumeName
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", h.ID, h.ResourceKind, resource, h.TargetID, h.Status, h.SizeBytes, h.StartedAt, h.FinishedAt)
	}
	_ = tw.Flush()
}

func backupsListAllUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups list-all [flags]

Lists backup attempt history across every database and app volume
instance-wide, newest first.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print backup history as a JSON array to stdout, nothing else
  --limit int              max attempts to return (default: server default)
  --before string          only show attempts started before this RFC3339 timestamp
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
