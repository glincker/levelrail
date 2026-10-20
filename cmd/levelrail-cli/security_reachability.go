package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runSecurityReachability(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var recheck bool
	var fixID string
	client, _, jsonOut, of, code, ok := securityCommand(prog, "security reachability", "print the report as JSON to stdout and nothing else", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&recheck, "recheck", false, "re-measure instead of using the cached audit")
		fs.StringVar(&fixID, "fix", "", "apply the automatic fix of one finding (its ID from the report)")
	})
	if !ok {
		return code
	}
	ctx := context.Background()
	if fixID != "" {
		return applyReachFix(ctx, client, fixID, stdout, stderr, jsonOut, of)
	}
	rep, err := client.GetSecurityReachability(ctx, recheck)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security reachability: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printReachability(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func applyReachFix(ctx context.Context, client *apiclient.Client, id string, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	rep, err := client.GetSecurityReachability(ctx, true)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security reachability: %w", err))
	}
	var found *apiclient.ReachFinding
	for i := range rep.Findings {
		if rep.Findings[i].ID == id {
			found = &rep.Findings[i]
		}
	}
	if found == nil || found.Fix == nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security reachability: no finding %q with a fix", id))
	}
	if err := client.ApplyReachFix(ctx, *found.Fix); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security reachability: fix %s: %w", id, err))
	}
	after, err := client.GetSecurityReachability(ctx, true)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security reachability: re-check: %w", err))
	}
	still := false
	for _, f := range after.Findings {
		still = still || f.ID == id
	}
	res := map[string]any{"id": id, "fixed": !still, "remaining": len(after.Findings)}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		state := "fixed"
		if still {
			state = "still failing"
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s (%d findings remain)\n", id, state, len(after.Findings))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printReachability(w io.Writer, rep apiclient.ReachReport) {
	_, _ = fmt.Fprintf(w, "Reachable from the internet: %d high, %d medium, %d low findings; %d ports narrowed by firewall rules\n",
		rep.Counts.High, rep.Counts.Medium, rep.Counts.Low, rep.RestrictedPorts)
	if !rep.RulesReadable {
		_, _ = fmt.Fprintln(w, "A node's firewall rules could not be read, so restricted ports may show as open.")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "\nSUBJECT\tNAME\tENDPOINTS")
	for _, s := range rep.Subjects {
		eps := ""
		for i, e := range s.Endpoints {
			if i > 0 {
				eps += ", "
			}
			eps += e.Value
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Type, s.Name, eps)
	}
	_ = tw.Flush()
	if len(rep.Findings) == 0 {
		return
	}
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "\nSEVERITY\tKIND\tSUBJECT\tFIX\tID")
	for _, f := range rep.Findings {
		fix := "none"
		if f.Fix != nil {
			fix = f.Fix.Kind
			if f.Fix.Link != "" {
				fix += " " + f.Fix.Link
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s/%s\t%s\t%s\n", f.Severity, f.Kind, f.Subject.Type, f.Subject.Name, fix, f.ID)
	}
	_ = tw.Flush()
}
