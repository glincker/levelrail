package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesEvents implements "nodes events <id>": GET
// /api/v1/nodes/{id}/events, a node's recent status transitions.
func runNodesEvents(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes events", "print events as a JSON array to stdout and nothing else", stderr)
	limit := fs.Int("limit", 0, "max events to return (default: server default)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesEventsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes events", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	events, err := client.ListNodeEvents(context.Background(), id, *limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list events for node %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, events, func() { printNodeEventsTable(stdout, events) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNodeEventsTable(out io.Writer, events []nodeStatusEventResource) {
	if len(events) == 0 {
		_, _ = fmt.Fprintln(out, "No status changes recorded yet.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TIME\tFROM\tTO")
	for _, e := range events {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", e.CreatedAt.Format("2006-01-02 15:04:05 MST"), e.FromStatus, e.ToStatus)
	}
	_ = tw.Flush()
}

func nodesEventsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes events <id> [--limit N] [flags]

Shows a node's recent status transitions (online, offline, cordoned),
newest first.

Flags:
  --limit int             max events to return, 1 to 200 (default: server default, 50)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print events as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
