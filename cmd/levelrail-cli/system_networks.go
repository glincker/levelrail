package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSystemNetworks implements "system networks [--orphans]" and
// "system networks prune --dry-run|--yes" (internal/api/system_networks.go).
func runSystemNetworks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	prune := len(args) > 0 && args[0] == "prune"
	label := "system networks"
	if prune {
		args = args[1:]
		label = "system networks prune"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	orphans := fs.Bool("orphans", false, "list only networks whose owner no longer exists")
	dryRun := fs.Bool("dry-run", false, "prune: show what would be removed without removing it")
	yes := fs.Bool("yes", false, "prune: remove the orphaned, empty networks")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s system networks [--orphans]\n  %[1]s system networks prune --dry-run|--yes\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if prune {
		if *dryRun == *yes {
			return reportError(stdout, stderr, jsonOut, newValidationError("pass exactly one of --dry-run or --yes"))
		}
		res, err := client.PruneSystemNetworks(ctx, *dryRun)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("prune networks: %w", err))
		}
		return finishRender(stdout, stderr, of, res, func() {
			verb, names := "removed", res.Removed
			if res.DryRun {
				verb, names = "would remove", res.Candidates
			}
			_, _ = fmt.Fprintf(stdout, "%s %d network(s)\n", verb, len(names))
			for _, n := range names {
				_, _ = fmt.Fprintf(stdout, "  %s\n", n)
			}
		})
	}

	res, err := client.GetSystemNetworks(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list networks: %w", err))
	}
	if *orphans {
		kept := res.Networks[:0:0]
		for _, n := range res.Networks {
			if n.Orphaned {
				kept = append(kept, n)
			}
		}
		res.Networks = kept
	}
	return finishRender(stdout, stderr, of, res, func() { printSystemNetworks(stdout, res) })
}

func finishRender(stdout, stderr io.Writer, of outputFlags, data any, human func()) int {
	if err := renderResult(stdout, of.Format, of.Query, data, human); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSystemNetworks(out io.Writer, r apiclient.SystemNetworks) {
	if r.Pool.Enabled {
		_, _ = fmt.Fprintf(out, "pool %s: %d of %d in use (%d%%)\n\n", r.Pool.CIDR, r.Pool.InUse, r.Pool.Capacity, r.Pool.Percent)
	} else {
		_, _ = fmt.Fprintln(out, "pool: disabled, Docker default address pools apply")
		_, _ = fmt.Fprintln(out)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tOWNER\tSUBNET\tCONTAINERS\tORPHANED\tSTATE")
	for _, n := range r.Networks {
		subnet := "-"
		if len(n.Subnets) > 0 {
			subnet = n.Subnets[0]
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s/%s\t%s\t%d\t%t\t%s\n", n.Name, n.OwnerKind, n.OwnerName, subnet, n.AttachedContainers, n.Orphaned, n.Reason)
	}
	_ = tw.Flush()
}
