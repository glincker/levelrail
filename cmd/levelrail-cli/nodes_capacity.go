package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runNodesCapacity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes capacity", "print the capacity as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes capacity <node> [flags]\n\nShows disk, inode, memory and swap headroom against the node's warn and critical thresholds.\nUse 'local' for the control plane's own node.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes capacity", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetNodeCapacity(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get capacity for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printNodeCapacity(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNodeCapacity(out io.Writer, r apiclient.NodeCapacityResource) {
	if !r.HasReading {
		_, _ = fmt.Fprintln(out, "No reading yet: the node has not reported its host numbers.")
		return
	}
	for _, m := range r.Status.Metrics {
		if m.Level == "unknown" {
			_, _ = fmt.Fprintf(out, "%-8s unknown\n", m.Name)
			continue
		}
		_, _ = fmt.Fprintf(out, "%-8s %-8s %5.1f%%\n", m.Name, m.Level, m.Percent)
	}
	_, _ = fmt.Fprintf(out, "Overall: %s\n", r.Status.Worst)
	if r.Status.Reason != "" {
		_, _ = fmt.Fprintln(out, r.Status.Reason)
	}
	if r.Status.Worst == "critical" {
		_, _ = fmt.Fprintln(out, "New builds and deploys on this node are blocked until space is freed.")
	}
}

func runNodesCleanup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes cleanup", "print the result as JSON to stdout and nothing else", stderr)
	dry := fs.Bool("dry-run", false, "preview what would be removed, remove nothing")
	yes := fs.Bool("yes", false, "remove the safe classes now")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes cleanup <node> --dry-run | --yes [flags]\n\nFrees space on the control plane's node: dangling images, build cache and stopped containers.\nRollback images and volumes are never removed.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes cleanup", "node id")
	if !ok {
		return exitUsage
	}
	if *dry == *yes {
		_, _ = fmt.Fprintln(stderr, "pass exactly one of --dry-run (preview) or --yes (remove)")
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.CleanupNode(context.Background(), id, *dry)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cleanup node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printNodeCleanup(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNodeCleanup(out io.Writer, r apiclient.NodeCleanupResult) {
	for _, c := range r.Preview.Classes {
		note := ""
		if !c.Deletable {
			note = " (never removed)"
		}
		_, _ = fmt.Fprintf(out, "%-20s %10s  %d item(s)%s\n", c.Name, formatRecommendationBytes(c.Bytes), len(c.Items), note)
	}
	if r.DryRun {
		_, _ = fmt.Fprintf(out, "Would free about %s. Run again with --yes to remove.\n", formatRecommendationBytes(r.Preview.TotalBytes))
		return
	}
	_, _ = fmt.Fprintf(out, "Freed %s.\n", formatRecommendationBytes(r.FreedBytes))
	if r.Error != "" {
		_, _ = fmt.Fprintf(out, "Partly failed: %s\n", r.Error)
	}
}
