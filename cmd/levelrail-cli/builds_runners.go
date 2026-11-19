package main

import (
	"flag"
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

// runBuildsRunner implements "builds runner <app>": show or set where one app builds.
func runBuildsRunner(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var mode, node string
	var pin bool
	var labels stringList
	c, code, ok := parseBuildsCall(prog, "builds runner", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&mode, "mode", "", "auto, control_plane or node (empty shows the current choice)")
		fs.StringVar(&node, "node", "", "build node ID or name, for mode node")
		fs.BoolVar(&pin, "pin", false, "never fall back to another runner")
		fs.Var(&labels, "label", "required runner label, key or key=value (repeatable)")
	})
	if !ok {
		return code
	}
	if len(c.rest) != 1 {
		return reportError(stdout, stderr, c.jsonO, newValidationError("builds runner needs exactly one app name"))
	}
	app := c.rest[0]
	var res apiclient.AppBuildRunner
	var err error
	if mode == "" && node == "" && !pin && len(labels) == 0 {
		res, err = c.client.GetAppBuildRunner(bg(), app)
	} else {
		if mode == "" {
			mode = "auto"
		}
		res, err = c.client.SetAppBuildRunner(bg(), app, apiclient.SetAppBuildRunnerRequest{Mode: mode, NodeID: node, Pinned: pin, Labels: labels})
	}
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("builds runner: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s builds: mode %s, node %q, pinned %t\n", res.App, res.Mode, res.NodeID, res.Pinned)
	})
}
