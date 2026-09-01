package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// runAuditLog implements "audit-log": GET /api/v1/audit-log
// (internal/api/audit.go), AbilityRoot-gated same as the API route
// itself. --format csv switches to the same endpoint's ?format=csv
// export, written to stdout or, with --output, to a file, for
// compliance/record-keeping use rather than live dashboard viewing.
func runAuditLog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "audit-log", "print audit log entries as a JSON array to stdout and nothing else", stderr)
	var limitFlag int
	var beforeFlag, pathFlag, methodFlag, formatFlag, outputFlag string
	fs.IntVar(&limitFlag, "limit", 0, "max entries to return (default: server default)")
	fs.StringVar(&beforeFlag, "before", "", "only show entries created before this RFC3339 timestamp (page backward using the TIME column of a prior run)")
	fs.StringVar(&pathFlag, "path", "", "only show entries for this exact request path")
	fs.StringVar(&methodFlag, "method", "", "only show entries for this exact HTTP method")
	fs.StringVar(&formatFlag, "format", "", `output format: "csv" exports entries as CSV instead of the default table/--json output`)
	fs.StringVar(&outputFlag, "output", "", "write the csv export to this file instead of stdout (only meaningful with --format csv)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, auditLogUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if formatFlag != "" && formatFlag != "csv" {
		_, _ = fmt.Fprintf(stderr, "%s: unsupported --format %q, only \"csv\" is supported\n\n", prog, formatFlag)
		fs.Usage()
		return exitUsage
	}
	if outputFlag != "" && formatFlag != "csv" {
		_, _ = fmt.Fprintf(stderr, "%s: --output requires --format csv\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	opts := listAuditLogOptions{Limit: limitFlag, Before: beforeFlag, Path: pathFlag, Method: methodFlag}

	if formatFlag == "csv" {
		return runAuditLogExportCSV(context.Background(), client, opts, outputFlag, stdout, stderr, jsonOut)
	}

	entries, err := client.ListAuditLog(context.Background(), opts)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list audit log: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, entries); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printAuditLogTable(stdout, entries)
	return exitOK
}

// runAuditLogExportCSV downloads the CSV export and writes it to
// outputPath, or to stdout when outputPath is empty.
func runAuditLogExportCSV(ctx context.Context, client *Client, opts listAuditLogOptions, outputPath string, stdout, stderr io.Writer, jsonOut bool) int {
	data, err := client.DownloadAuditLogCSV(ctx, opts)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export audit log csv: %w", err))
	}

	if outputPath == "" {
		if _, err := stdout.Write(data); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}

	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", outputPath, err))
	}
	if jsonOut {
		if err := writeJSONValue(stdout, map[string]string{"file": outputPath}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", outputPath)
	return exitOK
}

// printAuditLogTable prints a compact, aligned table of audit log
// entries, the same shape output.go's own list-command helpers already
// establish.
func printAuditLogTable(out io.Writer, entries []auditLogEntryResource) {
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "no audit log entries")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTIME\tACTOR\tABILITY\tMETHOD\tPATH\tSTATUS")
	for _, e := range entries {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s (%s)\t%s\t%s\t%s\t%d\n", e.ID, e.CreatedAt, e.ActorName, e.ActorType, e.Ability, e.Method, e.Path, e.StatusCode)
	}
	_ = tw.Flush()
}

func auditLogUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s audit-log [flags]

Lists every recorded write/deploy/root-tier request, newest first
(read-only requests aren't recorded). Requires an admin/root-scoped
token, the same as the underlying API route.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print entries as a JSON array to stdout, nothing else
  --limit int              max entries to return (default: server default)
  --before string          only show entries created before this RFC3339 timestamp
  --path string             only show entries for this exact request path
  --method string          only show entries for this exact HTTP method
  --format string          "csv" exports entries as CSV instead of the default table/--json output
  --output string          write the csv export to this file instead of stdout (requires --format csv)
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
