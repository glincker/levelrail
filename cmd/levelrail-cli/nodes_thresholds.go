package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runNodesThresholds implements "nodes thresholds get|set <id>".
func runNodesThresholds(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, nodesThresholdsUsage(prog))
		return exitUsage
	}
	action := args[0]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes thresholds "+action, "print the thresholds as JSON", stderr)
	var cpu, memory, disk string
	if action == "set" {
		fs.StringVar(&cpu, "cpu", "", "CPU alert percent for this node, 1 to 100, or \"default\"")
		fs.StringVar(&memory, "memory", "", "memory alert percent for this node, 1 to 100, or \"default\"")
		fs.StringVar(&disk, "disk", "", "disk alert percent for this node, 1 to 100, or \"default\"")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesThresholdsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes thresholds "+action, "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	res, err := client.GetNodeThresholds(ctx, id)
	if err == nil && action == "set" {
		next := res.Override
		for _, f := range []struct {
			raw string
			dst **int
		}{{cpu, &next.CPUPercent}, {memory, &next.MemoryPercent}, {disk, &next.DiskPercent}} {
			if err = applyThresholdFlag(f.raw, f.dst); err != nil {
				return reportError(stdout, stderr, jsonOut, err)
			}
		}
		res, err = client.SetNodeThresholds(ctx, id, next)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("thresholds of node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printNodeThresholds(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// applyThresholdFlag leaves the field alone when the flag is unset; "default" clears the override.
func applyThresholdFlag(raw string, dst **int) error {
	switch raw {
	case "":
		return nil
	case "default":
		*dst = nil
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return newValidationError("threshold %q must be a whole percent or \"default\"", raw)
	}
	*dst = &n
	return nil
}

func printNodeThresholds(out io.Writer, t apiclient.NodeThresholds) {
	row := func(name string, eff, ov *int) {
		src := "default"
		if ov != nil {
			src = "node override"
		}
		v := "unknown"
		if eff != nil {
			v = strconv.Itoa(*eff) + "%"
		}
		_, _ = fmt.Fprintf(out, "%-8s %-6s (%s)\n", name, v, src)
	}
	row("cpu", t.Effective.CPUPercent, t.Override.CPUPercent)
	row("memory", t.Effective.MemoryPercent, t.Override.MemoryPercent)
	row("disk", t.Effective.DiskPercent, t.Override.DiskPercent)
	_, _ = fmt.Fprintf(out, "sustained intervals: %d\n", t.SustainedIntervals)
}

func nodesThresholdsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes thresholds get <id> [flags]
  %[1]s nodes thresholds set <id> [--cpu N|default] [--memory N|default] [--disk N|default] [flags]

Shows or sets the alert percents node_cpu, node_memory and node_disk_space
rules use for one node. Unset values follow the platform defaults
(nodes.alert_* runtime settings); "default" removes a node override.
`, prog)
}
