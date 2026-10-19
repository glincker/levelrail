package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// runNodesZones implements "nodes zones": failure domains and their workloads.
func runNodesZones(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes zones", "print the zones as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "zones", "Shows each failure domain (zone label) with its nodes and workloads. Set zones with: nodes labels <id> zone=a region=eu provider=hetzner"))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetZones(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("node zones: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if !res.Labeled {
			_, _ = fmt.Fprintln(stdout, "no node has a zone label, so replicas are not spread across failure domains")
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ZONE\tREGION\tPROVIDER\tNODES\tONLINE\tWORKLOADS")
		for _, z := range res.Zones {
			zone := z.Zone
			if zone == "" {
				zone = "(unlabeled)"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\n", zone, z.Region, strings.Join(z.Providers, ","), len(z.NodeIDs), z.Online, z.Workloads)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
