package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runTemplatesShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates show", "print the requirements as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates show <id> [flags]\n\nShows what a template needs (privileges, published ports, secrets, volumes)\nand which Levelrail protections apply.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "templates show", "template id"}, lookupEnv)
	if !ok {
		return exitCode
	}

	template, err := client.GetServiceTemplate(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get service template %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, template.Requirements, func() { printTemplateRequirementsHuman(stdout, template) })
}

func printTemplateRequirementsHuman(out io.Writer, t serviceTemplateDetail) {
	r := t.Requirements
	_, _ = fmt.Fprintf(out, "%s (%s)\nlevel: %s\n", t.Name, t.ID, r.Level)
	_, _ = fmt.Fprintln(out, "\nWhat this needs:")
	if len(r.Findings) == 0 {
		_, _ = fmt.Fprintln(out, "  nothing beyond a standard container")
	}
	for _, f := range r.Findings {
		line := fmt.Sprintf("  [%s] %s", f.Severity, f.Message)
		if len(f.Services) > 0 {
			line += " (" + strings.Join(f.Services, ", ") + ")"
		}
		if f.Detail != "" {
			line += ": " + f.Detail
		}
		_, _ = fmt.Fprintln(out, line)
	}
	_, _ = fmt.Fprintln(out, "\nWhat Levelrail adds:")
	for _, p := range r.Protections {
		_, _ = fmt.Fprintf(out, "  - %s\n", p.Label)
	}
}
