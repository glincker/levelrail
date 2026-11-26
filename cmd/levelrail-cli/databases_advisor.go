package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var advisorSeverityRank = map[string]int{"info": 1, "warn": 2, "critical": 3}

func runDatabasesAdvisor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var severity string
	c, code, ok := parseDBAccess(prog, "databases advisor", "databases advisor <name> [--severity info|warn|critical] [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&severity, "severity", "", "show only findings at this severity or above: info, warn or critical")
	})
	if !ok {
		return code
	}
	minRank := 0
	if severity != "" {
		minRank = advisorSeverityRank[severity]
		if minRank == 0 {
			return c.fail(newValidationError("--severity must be info, warn or critical"))
		}
	}
	rep, err := c.client.GetDatabaseAdvisor(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("performance advisor for database %q: %w", c.rest[0], err))
	}
	kept := make([]apiclient.DatabaseAdvisorFinding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		if advisorSeverityRank[f.Severity] >= minRank {
			kept = append(kept, f)
		}
	}
	rep.Findings = kept
	return c.result(rep, func() { printAdvisor(stdout, rep) })
}

func printAdvisor(out io.Writer, rep apiclient.DatabaseAdvisor) {
	_, _ = fmt.Fprintf(out, "engine: %s    checked: %s    findings: %d\n", safeTerminalText(rep.Engine), safeTerminalText(rep.CheckedAt), len(rep.Findings))
	if rep.Truncated {
		_, _ = fmt.Fprintln(out, "more findings exist than the configured cap, raise databases.advisor_max_findings to see them")
	}
	for _, f := range rep.Findings {
		_, _ = fmt.Fprintf(out, "\n[%s] %s\n", safeTerminalText(f.Severity), safeTerminalText(f.Title))
		if f.Object != "" {
			_, _ = fmt.Fprintf(out, "  object: %s\n", safeTerminalText(f.Object))
		}
		keys := make([]string, 0, len(f.Evidence))
		for k := range f.Evidence {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(out, "  %s: %s\n", safeTerminalText(k), oneLine(fmt.Sprint(f.Evidence[k]), 200))
		}
		if f.Suggestion != "" {
			_, _ = fmt.Fprintf(out, "  suggestion: %s\n", oneLine(f.Suggestion, 400))
		}
		if f.Docs != "" {
			_, _ = fmt.Fprintf(out, "  docs: %s\n", safeTerminalText(f.Docs))
		}
	}
}
