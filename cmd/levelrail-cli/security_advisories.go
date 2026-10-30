package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSecurityAdvisories implements "security advisories list|match".
func runSecurityAdvisories(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	// No verb, or flags first, means list.
	mode, rest := "list", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		mode, rest = args[0], args[1:]
	}
	sub := mode
	switch sub {
	case "refresh":
		return runSecurityAdvisoryRefresh(prog, rest, stdout, stderr, lookupEnv)
	case "list", "match":
	default:
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		return exitUsage
	}
	client, _, jsonOut, of, code, ok := securityCommand(prog, "security advisories "+mode, "print the result as JSON to stdout and nothing else", rest, stderr, lookupEnv, nil)
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
	_, _ = fmt.Fprintln(tw, "APP\tIMAGE\tDIGEST\tADVISORY\tSEVERITY\tFIXED IN\tFIX")
	for _, m := range res.Matches {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", m.App, m.Image, dash(m.Digest), m.AdvisoryID, m.Severity, dash(m.FixedIn), m.Hint)
	}
}

// runSecurityAdvisoryRefresh implements "security advisories refresh <app>".
func runSecurityAdvisoryRefresh(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var confirm *bool
	client, fs, jsonOut, of, code, ok := securityCommand(prog, "security advisories refresh", "print the result as JSON to stdout and nothing else", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		confirm = fs.Bool("confirm", false, "acknowledge a protected environment (the deploy then waits for approval)")
	})
	if !ok {
		return code
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "usage: %s security advisories refresh <app> [--confirm] [flags]\n", prog)
		return exitUsage
	}
	res, err := client.RefreshAdvisoryApp(context.Background(), fs.Arg(0), *confirm)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security advisories refresh: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if res.PendingApproval != nil {
			_, _ = fmt.Fprintf(stdout, "Refresh of %s is waiting for approval (%s).\n", fs.Arg(0), res.PendingApproval.ID)
			return
		}
		_, _ = fmt.Fprintf(stdout, "Refresh of %s accepted: redeploying on the newest digest of its tag.\n", fs.Arg(0))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
