package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runResilience(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "check" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s resilience check [--simulate] [--fix] [flags]\n", prog)
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "resilience check", "print the report as JSON to stdout and nothing else", stderr)
	simulate := fs.Bool("simulate", false, "dry run: verify the persisted state would let ingress restart without the control plane (stops nothing)")
	fix := fs.Bool("fix", false, "rewrite the route snapshot from the applied config, then re-check")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s resilience check [--simulate] [--fix] [flags]\n\nShows whether the apps keep serving if the control plane is down.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var rep apiclient.OutageReport
	var err error
	switch {
	case *fix:
		rep, err = client.FixResilienceSnapshot(context.Background())
	case *simulate:
		rep, err = client.SimulateResilience(context.Background())
	default:
		rep, err = client.GetResilience(context.Background())
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("resilience check: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printResilience(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printResilience(out io.Writer, rep apiclient.OutageReport) {
	for _, s := range rep.Scopes {
		_, _ = fmt.Fprintf(out, "%s (%s)\n", s.Name, s.Kind)
		for _, c := range s.Checks {
			_, _ = fmt.Fprintf(out, "  %-8s %-18s %s\n", c.Status, c.ID, c.Detail)
			if c.Fix != "" {
				_, _ = fmt.Fprintln(out, "           fix available: run with --fix")
			}
		}
	}
	_, _ = fmt.Fprintf(out, "Overall: %s\n", rep.Overall)
}
