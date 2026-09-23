package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesResourceUsage implements "nodes resource-usage": GET
// /api/v1/nodes/resource-usage (internal/api/node_resource_usage.go),
// the node-scoped counterpart to "apps resource-usage". Table output
// prints one row per node plus a trailing FLEET summary row; --json
// preserves the server's own nodes+fleet shape for scripting.
func runNodesResourceUsage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes resource-usage", "print fleet resource usage as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes resource-usage [flags]\n\nLists every node's latest known CPU, memory, and disk usage, plus a\nfleet-wide rollup, the same numbers the dashboard's fleet summary shows.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	usage, err := client.GetFleetResourceUsage(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get fleet resource usage: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, usage, func() { printFleetResourceUsageTable(stdout, usage) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printFleetResourceUsageTable(out io.Writer, usage fleetResourceUsageResource) {
	if len(usage.Nodes) == 0 {
		_, _ = fmt.Fprintln(out, "no nodes")
		return
	}

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tCPU\tMEMORY\tDISK")
	for _, n := range usage.Nodes {
		local := ""
		if n.IsLocal {
			local = " (local)"
		}
		_, _ = fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\n",
			n.Name, local, formatCPUPercent(n.CPUPercent),
			formatMemoryUsage(n.MemoryUsageBytes, n.MemoryTotalBytes),
			formatDiskUsage(n.DiskUsedBytes, n.DiskTotalBytes))
	}
	_, _ = fmt.Fprintf(tw, "FLEET (%d nodes)\t%s\t%s\t%s\n",
		usage.Fleet.NodeCount, formatCPUPercent(usage.Fleet.TotalCPUPercent),
		formatFleetPercent(usage.Fleet.MemoryUsedPercent, usage.Fleet.NodesWithMemoryCapacity, usage.Fleet.NodeCount),
		formatFleetPercent(usage.Fleet.DiskUsedPercent, usage.Fleet.NodesWithDiskCapacity, usage.Fleet.NodeCount))
	_ = tw.Flush()
}

func formatDiskUsage(used, total *float64) string {
	if used == nil {
		return "-"
	}
	usedStr := formatRecommendationBytes(int64(*used))
	if total == nil || *total <= 0 {
		return usedStr
	}
	return fmt.Sprintf("%s / %s", usedStr, formatRecommendationBytes(int64(*total)))
}

// formatFleetPercent renders a rollup percentage alongside how many of
// the fleet's nodes actually contributed a capacity reading, the same
// coverage caveat FleetResourceUsageRollup's own doc comment requires
// callers to surface rather than showing the percentage as if it covered
// every node.
func formatFleetPercent(pct *float64, nodesWithCapacity, nodeCount int) string {
	if pct == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f%% (%d/%d nodes)", *pct, nodesWithCapacity, nodeCount)
}
