package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsEgressReport is a read only dry run: it enforces nothing.
func runAppsEgressReport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps egress report", "print the report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps egress report <name> [flags]\n\nLists names the app asked the mesh DNS server for and which of them its\negress allowlist would deny. Needs dns.query_log_enabled.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps egress report", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	result, err := client.AppEgressReport(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("egress report for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printEgressReport(stdout, result) })
}

func printEgressReport(w io.Writer, r apiclient.EgressReport) {
	if r.Message != "" {
		_, _ = fmt.Fprintln(w, r.Message)
		return
	}
	if len(r.Names) == 0 {
		_, _ = fmt.Fprintf(w, "no names recorded for %s yet\n", r.App)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tCOUNT\tLAST SEEN\tVERDICT")
	for _, n := range r.Names {
		verdict := "allowed"
		if n.WouldDeny {
			verdict = "would be denied"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", n.Name, n.Count, n.LastSeen.Format("15:04:05"), verdict)
	}
	_ = tw.Flush()
	if r.Truncated > 0 {
		_, _ = fmt.Fprintf(w, "%d more names not shown (dns.egress_report_max_names)\n", r.Truncated)
	}
}
