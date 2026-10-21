package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const diagnoseUsageText = `Usage:
  %s diagnose <app|database|domain|node> <name> [flags]

Explains why a resource is not ready: how long it has been that way, the
ranked likely causes with the evidence behind each, and the next actions.
Deterministic rules over recorded conditions; never calls an external model.
For a failed app deploy in detail, see "apps diagnose"; for DNS and
certificates, see "domains diagnose".

Flags:
`

// runDiagnose implements "diagnose <kind> <name>".
func runDiagnose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "diagnose", "print the diagnosis as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, diagnoseUsageText, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "diagnose", "a kind (app, database, domain, node) and a name", 2)
	if !ok {
		return exitUsage
	}
	kind, name := strings.ToLower(rest[0]), rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rep, err := client.DiagnoseStuck(context.Background(), kind, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("diagnose %s %q: %w", kind, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printStuckReport(stdout, rep) })
}

func printStuckReport(out io.Writer, r apiclient.StuckReport) {
	if r.Ready {
		_, _ = fmt.Fprintf(out, "%s %s is ready.\n", r.Kind, r.Name)
		return
	}
	state := "not ready"
	if r.Stuck {
		state = "stuck"
	}
	_, _ = fmt.Fprintf(out, "%s %s is %s (not ready for %s)\n", r.Kind, r.Name, state, (time.Duration(r.NotReadySeconds) * time.Second).String())
	if len(r.Timeline) > 0 {
		_, _ = fmt.Fprintln(out, "\ntimeline:")
		for _, c := range r.Timeline {
			_, _ = fmt.Fprintf(out, "  %s  %s=%s (%s)\n", c.Since.Local().Format("2006-01-02 15:04:05"), c.Type, c.Status, c.Reason)
		}
	}
	for i, c := range r.Causes {
		_, _ = fmt.Fprintf(out, "\n%d. %s [%s]\n   %s\n", i+1, c.Title, c.Confidence, c.Explanation)
		for _, e := range c.Evidence {
			_, _ = fmt.Fprintf(out, "   evidence: %s\n", e)
		}
		for _, a := range c.Actions {
			line := a.Label
			switch {
			case a.FixID != "":
				line += " (" + "fixes apply " + a.FixID + ")"
			case a.Href != "":
				line += " (" + a.Href + ")"
			}
			_, _ = fmt.Fprintf(out, "   next: %s\n", line)
		}
	}
}
