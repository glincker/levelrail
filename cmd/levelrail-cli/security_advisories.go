package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSecurityAdvisories implements "security advisories list|match".
func runSecurityAdvisories(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "list" && args[0] != "match") {
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		return exitUsage
	}
	mode := args[0]
	client, _, jsonOut, of, code, ok := securityCommand(prog, "security advisories "+mode, "print the result as JSON to stdout and nothing else", args[1:], stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	res, err := client.GetAdvisories(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security advisories: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printAdvisories(stdout, mode, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAdvisories(w io.Writer, mode string, res apiclient.AdvisoriesResource) {
	if !res.Enabled {
		_, _ = fmt.Fprintln(w, "No advisory feed is configured (set APP_ADVISORY_FEED on the server).")
		return
	}
	_, _ = fmt.Fprintf(w, "Feed %s, signature: %s, %d advisories\n", res.Source, res.Signature, len(res.Entries))
	if res.Error != "" {
		_, _ = fmt.Fprintf(w, "Feed not usable: %s\n", res.Error)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	defer func() { _ = tw.Flush() }()
	if mode == "list" {
		_, _ = fmt.Fprintln(tw, "ID\tSEVERITY\tFIXED IN\tSUMMARY")
		for _, e := range res.Entries {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.ID, e.Severity, e.FixedIn, e.Summary)
		}
		return
	}
	if len(res.Matches) == 0 {
		_, _ = fmt.Fprintln(tw, "No running image matches a known advisory.")
		return
	}
	_, _ = fmt.Fprintln(tw, "APP\tIMAGE\tADVISORY\tSEVERITY\tFIX")
	for _, m := range res.Matches {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.App, m.Image, m.AdvisoryID, m.Severity, m.Hint)
	}
}
