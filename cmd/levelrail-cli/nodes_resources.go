package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesResources implements "nodes resources <id>": the apps and databases placed on a node.
func runNodesResources(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes resources", "print the node's resources as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "resources <id>", "Lists the apps and databases placed on a node, with kind, size and status. A drain would move every row."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes resources", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ListNodeResources(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list resources of node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if len(res.Resources) == 0 {
			_, _ = fmt.Fprintln(stdout, "nothing is placed on this node")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tNAME\tDETAIL\tMEMORY\tCPU\tSTATUS")
		for _, r := range res.Resources {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Kind, r.Name, r.Detail, nodeResourceMemory(r.MemoryBytes), nodeResourceCPU(r.NanoCPUs), r.Status)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodeResourceMemory(b int64) string {
	if b <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dMi", b>>20)
}

func nodeResourceCPU(n int64) string {
	if n <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", float64(n)/1e9)
}
