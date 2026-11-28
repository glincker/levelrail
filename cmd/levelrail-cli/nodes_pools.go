package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/placement"
)

// runNodesPools implements "nodes pools [set <name> key=value ... | delete <name>]".
func runNodesPools(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes pools", "print the pools as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s nodes pools\n  %[1]s nodes pools set <name> key=value ...\n  %[1]s nodes pools delete <name>\n\nA pool names a node label selector; apps target it with 'apps placement <name> --pool <pool>'.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	rest := fs.Args()
	switch {
	case len(rest) == 0:
		pools, err := client.ListNodePools(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list node pools: %w", err))
		}
		if err := renderResult(stdout, of.Format, of.Query, pools, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "POOL\tSELECTOR")
			for _, p := range pools {
				_, _ = fmt.Fprintf(tw, "%s\t%s\n", p.Name, placement.Selector(p.Selector).String())
			}
			_ = tw.Flush()
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	case rest[0] == "set" && len(rest) >= 2:
		sel, err := placement.ParseLabels(rest[2:])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		p, err := client.SetNodePool(ctx, rest[1], sel)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set node pool %q: %w", rest[1], err))
		}
		if err := renderResult(stdout, of.Format, of.Query, p, func() {
			_, _ = fmt.Fprintf(stdout, "pool %s: %s\n", p.Name, placement.Selector(p.Selector).String())
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	case rest[0] == "delete" && len(rest) == 2:
		if err := client.DeleteNodePool(ctx, rest[1]); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete node pool %q: %w", rest[1], err))
		}
		_, _ = fmt.Fprintf(stdout, "pool %s deleted\n", rest[1])
		return exitOK
	}
	fs.Usage()
	return exitUsage
}
