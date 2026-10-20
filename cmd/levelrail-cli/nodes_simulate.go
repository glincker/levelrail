package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func simulatedRows(stdout io.Writer, rows []apiclient.SimulatedPlacement) {
	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WORKLOAD\tFROM\tTO\tZONE\tWHY")
	for _, p := range rows {
		to, why := p.NodeID, p.Why
		if p.Unplaced != "" {
			to, why = "(none)", p.Unplaced
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.Name, orDash(p.From), to, orDash(p.Zone), why)
	}
	_ = tw.Flush()
}

// runNodesSimulate implements "nodes simulate": a dry run of adding apps.
func runNodesSimulate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes simulate", "print the simulation as JSON", stderr)
	cpu := fs.Int64("cpu", 0, "CPU request per app in millicores")
	mem := fs.Int64("memory", 0, "memory request per app in MiB")
	count := fs.Int("count", 1, "how many identical apps to add")
	selector := fs.String("selector", "", "required node labels, key=value,key=value")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "simulate [--cpu M] [--memory MiB] [--count N] [--selector k=v]", "Shows which node and zone each new app would land on, and why one is refused. Nothing is created."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SimulatePlacement(context.Background(), *cpu, *mem, *count, *selector)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("simulate placement: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		simulatedRows(stdout, res.Placed)
		_, _ = fmt.Fprintf(stdout, "%d placed, %d refused\n", len(res.Placed)-res.Unplaced, res.Unplaced)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesFailure implements "nodes failure": what losing a zone or node does.
func runNodesFailure(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes failure", "print the simulation as JSON", stderr)
	zone := fs.String("zone", "", "zone to take down")
	node := fs.String("node", "", "node id to take down")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "failure --zone Z | --node ID", "Shows where every workload on the lost nodes would be rescheduled, and which would have nowhere to go. Nothing is moved."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if (*zone == "") == (*node == "") {
		_, _ = fmt.Fprintln(stderr, "pass exactly one of --zone or --node")
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SimulateFailure(context.Background(), *zone, *node)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("simulate failure: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "%d node(s) lost, %d workload(s) displaced, %d with nowhere to go\n", len(res.Lost), res.Count, res.Unplaced)
		if res.Count > 0 {
			simulatedRows(stdout, res.Placed)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
