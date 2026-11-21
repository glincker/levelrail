package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesTargets implements "nodes targets": where a workload can be deployed, grouped by cloud account.
func runNodesTargets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes targets", "print the targets as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "targets", "Lists deploy targets grouped by cloud account, then this server and unmanaged nodes. Use a node id as the placement target."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ListDeployTargets(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("node targets: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "GROUP\tPROVIDER\tNODE\tID\tHEALTH\tREGION")
		for _, g := range res {
			for _, n := range g.Nodes {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", g.Label, g.Provider, n.Name, n.ID, n.Health, n.Region)
			}
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
