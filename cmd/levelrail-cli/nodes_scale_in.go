package main

import (
	"context"
	"fmt"
	"io"

	"github.com/glincker/levelrail/internal/nodescale"
)

// Scale-in is dry run only in this release, so the flag is required to make that explicit.
func runNodesScaleIn(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes scale-in", "print the preview as JSON to stdout and nothing else", stderr)
	dryRun := fs.Bool("dry-run", false, "list idle nodes that would be removed (required, nothing is destroyed)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesScaleInUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if !*dryRun {
		_, _ = fmt.Fprintln(stderr, "nodes scale-in is dry run only in this release: pass --dry-run")
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	p, err := client.GetNodeScaleInPreview(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get scale-in preview: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, p, func() { printScaleInHuman(stdout, p) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printScaleInHuman(out io.Writer, p nodescale.Preview) {
	if !p.Enabled {
		_, _ = fmt.Fprintln(out, "nodes.scale_in is off: no would_destroy events are recorded. Showing what it would find now.")
	}
	if len(p.Candidates) == 0 {
		_, _ = fmt.Fprintf(out, "No node has been idle for %d minutes.\n", p.IdleMinutes)
		return
	}
	for _, c := range p.Candidates {
		price := "price unknown"
		if c.MonthlyPrice != nil {
			price = fmt.Sprintf("%.2f per month", *c.MonthlyPrice)
		}
		_, _ = fmt.Fprintf(out, "%s (%s) pool %s idle %d min, %s\n", c.NodeName, c.NodeID, c.Pool, c.IdleMinutes, price)
	}
	_, _ = fmt.Fprintf(out, "Monthly saving: %.2f", p.MonthlySaving)
	if p.UnpricedCandidate > 0 {
		_, _ = fmt.Fprintf(out, " (%d without a price)", p.UnpricedCandidate)
	}
	_, _ = fmt.Fprintln(out, "\nDry run only: nothing was destroyed.")
}

func nodesScaleInUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes scale-in --dry-run [flags]

Lists idle cloud nodes a scale-in would remove and the monthly saving.
A node qualifies when Levelrail created it, it carries the label
autoscale=on, it has run no apps or databases for nodes.scale_in_idle_minutes,
it is not the last node in its pool, not the control plane and does not
hold the only copy of a volume. Scale-in is dry run only in this release.

Flags:
  --dry-run               required; nothing is destroyed
  --token string          API token
  --api-url string       control plane base URL
  --profile string       named credentials profile
  --json                    print JSON to stdout, nothing else
`, prog)
}
