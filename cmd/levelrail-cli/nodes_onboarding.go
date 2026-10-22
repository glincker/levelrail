package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	defaultPreflightTimeout  = 10 * time.Minute
	defaultPreflightInterval = 3 * time.Second
)

// waitForNodeOnboarding polls until a node called name exists and has
// reported its facts, or ctx expires. progress gets one line per stage change.
func waitForNodeOnboarding(ctx context.Context, client *apiclient.Client, name string, interval time.Duration, progress io.Writer) (apiclient.NodeOnboarding, error) {
	last := ""
	say := func(s string) {
		if s != last {
			_, _ = fmt.Fprintln(progress, s)
			last = s
		}
	}
	var node apiclient.NodeResource
	for {
		if node.ID == "" {
			nodes, err := client.ListNodes(ctx)
			if err != nil && ctx.Err() == nil {
				return apiclient.NodeOnboarding{}, err
			}
			for _, n := range nodes {
				if n.Name == name {
					node = n
				}
			}
			if node.ID == "" {
				say("waiting for the agent to contact the control plane")
			}
		}
		if node.ID != "" {
			ob, err := client.GetNodeOnboarding(ctx, node.ID)
			if err != nil && ctx.Err() == nil {
				return ob, err
			}
			switch {
			case ob.Done():
				return ob, nil
			case ob.Stage == "connected":
				say("agent connected over mTLS, waiting for its preflight facts")
			default:
				say("agent enrolled, waiting for it to connect")
			}
		}
		select {
		case <-ctx.Done():
			return apiclient.NodeOnboarding{}, fmt.Errorf("node %q did not finish onboarding: %w", name, ctx.Err())
		case <-time.After(interval):
		}
	}
}

func printOnboarding(out io.Writer, ob apiclient.NodeOnboarding) {
	d := ob.Domain
	if ob.Detected != nil && ob.Detected.Source != "" {
		_, _ = fmt.Fprintf(out, "detected: provider=%s region=%s zone=%s (from %s)\n", dash(ob.Detected.Provider), dash(ob.Detected.Region), dash(ob.Detected.Zone), ob.Detected.Source)
	}
	_, _ = fmt.Fprintf(out, "labels:   provider=%s region=%s zone=%s\n\n", dash(d.Provider), dash(d.Region), dash(d.Zone))
	if len(ob.Checks) == 0 {
		_, _ = fmt.Fprintf(out, "no preflight facts yet (stage: %s)\n", ob.Stage)
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, c := range ob.Checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.ID, strings.ToUpper(c.Status), c.Detail)
	}
	_ = tw.Flush()
	for _, c := range ob.Checks {
		if c.Status == "pass" || c.Fix == "" {
			continue
		}
		_, _ = fmt.Fprintf(out, "\n%s (%s): %s\n", c.ID, c.Status, c.Fix)
		if c.FixCommand != "" {
			_, _ = fmt.Fprintf(out, "  run on the node: %s\n", c.FixCommand)
		}
	}
	_, _ = fmt.Fprintf(out, "\n%d passed, %d warnings, %d failed, %d unverified\n", ob.Summary.Pass, ob.Summary.Warn, ob.Summary.Fail, ob.Summary.Unknown)
}

// runNodesPreflight implements "nodes preflight <id-or-name> [--wait]".
func runNodesPreflight(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes preflight", "print the preflight result as JSON", stderr)
	var wait bool
	var timeout, interval time.Duration
	fs.BoolVar(&wait, "wait", false, "wait until the node has reported its facts")
	fs.DurationVar(&timeout, "timeout", defaultPreflightTimeout, "how long --wait waits")
	fs.DurationVar(&interval, "poll-interval", defaultPreflightInterval, "how often --wait polls")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "preflight <node-id-or-name> [--wait]",
			"Shows the node's detected provider, region and zone and its preflight checks (Docker,\nclock skew, disk, kernel WireGuard, architecture, mesh port), each failure with a fix.\nExits non-zero when a check failed. --timeout (default 10m) and --poll-interval\n(default 3s) control how long and how often --wait polls."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos, ok := requireArgs(fs, stderr, prog, "nodes preflight", "<node-id-or-name>", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	nodes, err := client.ListNodes(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list nodes: %w", err))
	}
	id := pos[0]
	for _, n := range nodes {
		if n.Name == pos[0] {
			id = n.ID
		}
	}
	var ob apiclient.NodeOnboarding
	if wait {
		wctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		name := pos[0]
		for _, n := range nodes {
			if n.ID == pos[0] {
				name = n.Name
			}
		}
		ob, err = waitForNodeOnboarding(wctx, client, name, interval, stderr)
	} else {
		ob, err = client.GetNodeOnboarding(ctx, id)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("node preflight: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, ob, func() { printOnboarding(stdout, ob) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if ob.Summary.Fail > 0 {
		return exitAPIError
	}
	return exitOK
}

// runNodesDomains implements "nodes domains".
func runNodesDomains(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes domains", "print the overview as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "domains",
			"Shows reported capacity per provider, region and zone, and apps whose replicas all\nsit in one failure domain."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	d, err := client.GetNodeDomains(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("node domains: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, d, func() { printDomains(stdout, d) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printDomains(out io.Writer, d apiclient.NodeDomains) {
	for _, sec := range []struct {
		title  string
		groups []apiclient.NodeDomainGroup
	}{{"PROVIDER", d.Providers}, {"REGION", d.Regions}, {"ZONE", d.Zones}} {
		if len(sec.groups) == 0 {
			continue
		}
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintf(tw, "%s\tNODES\tONLINE\tWORKLOADS\tCPU\tMEMORY\tUNVERIFIED\n", sec.title)
		for _, g := range sec.groups {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.1f cores\t%.1f GiB\t%d\n", g.Name, g.Nodes, g.Online, g.Workloads, float64(g.CPUMilli)/1000, float64(g.MemoryBytes)/(1<<30), g.Unverified)
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(out)
	}
	if d.Unlabeled > 0 {
		_, _ = fmt.Fprintf(out, "%d of %d nodes have no provider, region or zone label\n", d.Unlabeled, d.Total)
	}
	for _, r := range d.Risks {
		_, _ = fmt.Fprintf(out, "%s: %s\n", strings.ToUpper(r.Level), r.Message)
	}
}
