package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runBuildsRunners implements "builds runners": every build-capable node with its capacity and health.
func runBuildsRunners(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseBuildsCall(prog, "builds runners", args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rs, err := c.client.GetBuildRunners(bg())
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("builds runners: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, rs, func() { printBuildRunners(stdout, rs) })
}

func printBuildRunners(stdout io.Writer, rs apiclient.BuildRunners) {
	_, _ = fmt.Fprintf(stdout, "Runner choice: %s\n", rs.Strategy)
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSTATUS\tSLOTS\tWAITING\tPRESSURE\tARCH\tLABELS\tLAST BUILD")
	for _, r := range rs.Runners {
		slots := "-"
		if r.SlotsTotal > 0 {
			slots = fmt.Sprintf("%d/%d", r.SlotsUsed, r.SlotsTotal)
		}
		last := "-"
		if r.LastBuildAt != nil {
			last = r.LastBuildApp + " " + r.LastBuildAt.Local().Format("2006-01-02 15:04")
		}
		labels := "-"
		if len(r.Labels) > 0 {
			labels = strings.Join(r.Labels, ",")
		}
		arch := r.Architecture
		if arch == "" {
			arch = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n", r.Name, r.Status, slots, r.Waiting, r.Pressure, arch, labels, last)
	}
	_ = tw.Flush()
}
