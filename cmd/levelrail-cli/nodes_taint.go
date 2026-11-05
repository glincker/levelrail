package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// formatTaint renders a taint as key=value:Effect, dropping an empty value.
func formatTaint(t apiclient.NodeTaint) string {
	if t.Value == "" {
		return t.Key + ":" + t.Effect
	}
	return t.Key + "=" + t.Value + ":" + t.Effect
}

// parseTaints reads key[=value]:Effect arguments; the server validates the
// key, value and effect, so this only splits.
func parseTaints(args []string) ([]apiclient.NodeTaint, error) {
	out := make([]apiclient.NodeTaint, 0, len(args))
	for _, a := range args {
		i := strings.LastIndex(a, ":")
		if i <= 0 || i == len(a)-1 {
			return nil, fmt.Errorf("taint %q must look like key[=value]:Effect", a)
		}
		key, value, _ := strings.Cut(a[:i], "=")
		out = append(out, apiclient.NodeTaint{Key: key, Value: value, Effect: a[i+1:]})
	}
	return out, nil
}

// runNodesTaint implements "nodes taint <id> [key[=value]:Effect ...] [--clear]":
// with no taints it shows them, with taints it replaces the set.
func runNodesTaint(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes taint", "print taints as JSON", stderr)
	var clearAll bool
	fs.BoolVar(&clearAll, "clear", false, "remove every taint")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "taint <id> [key[=value]:Effect ...] [--clear]",
			"Shows a node's taints, or replaces them with the given ones. Effect is NoSchedule,\nPreferNoSchedule or NoExecute. Apps are kept off a tainted node unless they tolerate it."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: nodes taint needs a node id\n", prog)
		return exitUsage
	}
	id, specs := fs.Arg(0), fs.Args()[1:]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.NodeTaints
	var err error
	if len(specs) == 0 && !clearAll {
		res, err = client.GetNodeTaints(ctx, id)
	} else {
		taints, perr := parseTaints(specs)
		if perr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, perr)
			return exitUsage
		}
		res, err = client.SetNodeTaints(ctx, id, taints)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("node taints %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if len(res.Taints) == 0 {
			_, _ = fmt.Fprintln(stdout, "no taints")
		}
		for _, t := range res.Taints {
			_, _ = fmt.Fprintf(stdout, "  %s\n", formatTaint(t))
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
