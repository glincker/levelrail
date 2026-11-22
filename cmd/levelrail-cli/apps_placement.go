package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/placement"
)

func dimText(d apiclient.PlacementDimension, v int64) string {
	if d.Name == placement.DimCPU {
		return placement.FormatCPU(v)
	}
	return placement.FormatBytes(v)
}

func dimSummary(dims []apiclient.PlacementDimension) string {
	out := ""
	for i, d := range dims {
		if i > 0 {
			out += "  "
		}
		if d.Allocatable == nil {
			out += d.Name + " unknown"
			continue
		}
		out += fmt.Sprintf("%s %s/%s", d.Name, dimText(d, d.Requested), dimText(d, *d.Allocatable))
	}
	return out
}

func printPlacementDecision(out io.Writer, title string, d apiclient.PlacementDecision) {
	if d.NodeID == "" && len(d.Considered) == 0 {
		_, _ = fmt.Fprintf(out, "%s: none\n", title)
		return
	}
	_, _ = fmt.Fprintf(out, "%s: %s\n  why: %s\n", title, nodeLabel(d.NodeID), d.Why)
	for _, w := range d.Warnings {
		_, _ = fmt.Fprintf(out, "  warning: %s\n", w)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NODE\tRESULT\tDETAIL")
	for _, e := range d.Considered {
		result, detail := "fits", dimSummary(e.Dimensions)
		switch {
		case !e.Eligible:
			result, detail = "refused", e.Reason
		case e.Unverified:
			result = "unverified"
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", nodeLabel(e.NodeID), result, detail)
	}
	_ = tw.Flush()
}

// runAppsPlacement implements "apps placement <name>": why the app runs where it does.
func runAppsPlacement(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps placement", "print the placement decision as JSON", stderr)
	var db bool
	fs.BoolVar(&db, "database", false, "the name is a database, not an app")
	var pool, warmNodes, warmLabels string
	var clearPool, clearWarm bool
	fs.StringVar(&pool, "pool", "", "target this node pool (see 'nodes pools')")
	fs.BoolVar(&clearPool, "clear-pool", false, "stop targeting a pool")
	fs.StringVar(&warmNodes, "warm-node", "", "comma-separated node ids that keep the app's image pulled")
	fs.StringVar(&warmLabels, "warm-label", "", "comma-separated key=value labels; nodes with all of them keep the image pulled")
	fs.BoolVar(&clearWarm, "clear-warm", false, "stop keeping the image pulled anywhere")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, `Usage:
  %[1]s apps placement <name> [--database] [--pool <pool> | --clear-pool]
      [--warm-node a,b] [--warm-label k=v] [--clear-warm] [flags]

Shows why an app runs on its node: which nodes were considered, which were
refused and the shortfall, and a fresh evaluation of every node right now.

Flags:
`, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps placement requires an app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	kind, name := "app", fs.Arg(0)
	if db {
		kind = "database"
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if code, handled := applyAppPlacementEdits(client, name, db, pool, clearPool, warmNodes, warmLabels, clearWarm, stdout, stderr, jsonOut); handled {
		return code
	}
	view, err := client.GetPlacementDecision(context.Background(), kind, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("placement decision for %s %q: %w", kind, name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, view, func() {
		_, _ = fmt.Fprintf(stdout, "%s %s runs on %s\n", view.Kind, view.Name, nodeLabel(view.NodeID))
		if view.Recorded == nil {
			_, _ = fmt.Fprintln(stdout, "recorded decision: none (placed before decisions were recorded, or on the local node by default)")
		} else {
			printPlacementDecision(stdout, fmt.Sprintf("recorded decision (%s, %s)", view.Recorded.Trigger, view.Recorded.DecidedAt.Format("2006-01-02 15:04")), view.Recorded.Decision)
		}
		printPlacementDecision(stdout, "if placed now", view.Preview)
		for _, w := range view.Warnings {
			_, _ = fmt.Fprintf(stdout, "warning: %s\n", w)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesAllocations implements "nodes allocations": requested versus allocatable per node.
func runNodesAllocations(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes allocations", "print the allocations as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "allocations", "Shows each node's CPU, memory and disk that placed workloads request against what the node can allocate, and the overcommit policy in force."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetNodeAllocations(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("node allocations: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		p := res.Policy
		_, _ = fmt.Fprintf(stdout, "policy: cpu %d%%, memory %d%%, keep %d%% disk free, reject unknown capacity: %t\n", p.CPUOvercommitPercent, p.MemoryOvercommitPercent, p.MinFreeDiskPercent, p.RejectUnknown)
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NODE\tWORKLOADS\tREQUESTED/ALLOCATABLE")
		for _, n := range res.Nodes {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\n", n.Name, n.Workloads, dimSummary(n.Dimensions))
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// applyAppPlacementEdits applies --pool and warm flags; handled is false when
// none were given so the caller prints the decision as usual.
func applyAppPlacementEdits(client *apiclient.Client, name string, isDB bool, pool string, clearPool bool, warmNodes, warmLabels string, clearWarm bool, stdout, stderr io.Writer, jsonOut bool) (int, bool) {
	warmSet := warmNodes != "" || warmLabels != "" || clearWarm
	if pool == "" && !clearPool && !warmSet {
		return 0, false
	}
	if isDB {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("pool and warm flags apply to apps only")), true
	}
	ctx := context.Background()
	if pool != "" || clearPool {
		if _, err := client.SetAppPool(ctx, name, pool); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set pool for app %q: %w", name, err)), true
		}
		_, _ = fmt.Fprintf(stdout, "app %s pool: %s\n", name, noneIfEmpty(pool))
	}
	if warmSet {
		t := apiclient.WarmTargets{Nodes: []string{}, Labels: map[string]string{}}
		if warmNodes != "" {
			t.Nodes = strings.Split(warmNodes, ",")
		}
		if warmLabels != "" {
			l, err := placement.ParseLabels(strings.Split(warmLabels, ","))
			if err != nil {
				return reportError(stdout, stderr, jsonOut, err), true
			}
			t.Labels = l
		}
		got, err := client.SetAppWarm(ctx, name, t)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set warm targets for app %q: %w", name, err)), true
		}
		_, _ = fmt.Fprintf(stdout, "app %s image kept pulled on nodes [%s] labels %s\n", name, strings.Join(got.Nodes, ","), noneIfEmpty(placement.Selector(got.Labels).String()))
	}
	return exitOK, true
}
