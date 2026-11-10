package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const incidentsPostmortemUsage = `Usage:
  %[1]s incidents postmortem show <id> [--json]
  %[1]s incidents postmortem draft <id>
  %[1]s incidents postmortem edit <id> [--summary <text>] [--impact <text>] [--root-cause <text>] [--resolution <text>] [--action-item <text>]...
  %[1]s incidents postmortem publish <id>
`

// runIncidentsPostmortem dispatches the postmortem verbs. Publish is final: the server refuses every later edit.
func runIncidentsPostmortem(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, incidentsPostmortemUsage, prog)
		return 2
	}
	rest := args[1:]
	stateLine := func(st apiclient.PostmortemState) func() {
		return func() {
			_, _ = fmt.Fprintf(stdout, "Postmortem for %s is %s\n", safeTerminalText(st.IncidentID), safeTerminalText(st.State))
		}
	}
	switch args[0] {
	case "show":
		return runIncidentsID(prog, "postmortem show", rest, stdout, stderr, lookupEnv, func(c *apiclient.Client, id string) (any, func(), error) {
			p, err := c.GetIncidentPostmortem(context.Background(), id)
			return p, func() { printPostmortem(stdout, p) }, err
		})
	case "draft":
		return runIncidentsID(prog, "postmortem draft", rest, stdout, stderr, lookupEnv, func(c *apiclient.Client, id string) (any, func(), error) {
			st, err := c.DraftIncidentPostmortem(context.Background(), id)
			return st, stateLine(st), err
		})
	case "publish":
		return runIncidentsID(prog, "postmortem publish", rest, stdout, stderr, lookupEnv, func(c *apiclient.Client, id string) (any, func(), error) {
			st, err := c.PublishIncidentPostmortem(context.Background(), id)
			return st, stateLine(st), err
		})
	case "edit":
		return runIncidentsPostmortemEdit(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "unknown incidents postmortem command %q\n", args[0])
	return 2
}

func runIncidentsPostmortemEdit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "incidents postmortem edit", "print the result as JSON to stdout and nothing else", stderr)
	summary := fs.String("summary", "", "what happened, in a few sentences")
	impact := fs.String("impact", "", "who and what was affected")
	rootCause := fs.String("root-cause", "", "why it happened")
	resolution := fs.String("resolution", "", "how it was fixed")
	var items stringListFlag
	fs.Var(&items, "action-item", "follow-up task (repeatable; replaces the saved list)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s incidents postmortem edit <id> [--summary <text>] [--impact <text>] [--root-cause <text>] [--resolution <text>] [--action-item <text>]... [flags]\n\nOnly the flags you pass change. Refused once the postmortem is published.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, stderr, singleArgCmd{prog, "incidents postmortem edit", "incident id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	var req apiclient.PostmortemEditRequest
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "summary":
			req.Summary = summary
		case "impact":
			req.Impact = impact
		case "root-cause":
			req.RootCause = rootCause
		case "resolution":
			req.Resolution = resolution
		case "action-item":
			list := []string(items)
			req.ActionItems = &list
		}
	})
	st, err := client.EditIncidentPostmortem(context.Background(), id, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents postmortem edit %q: %w", id, err))
	}
	return writeIncidentResult(stdout, stderr, of, st, func() {
		_, _ = fmt.Fprintf(stdout, "Postmortem for %s saved (%s)\n", safeTerminalText(st.IncidentID), safeTerminalText(st.State))
	})
}

func printPostmortem(w io.Writer, p apiclient.IncidentPostmortem) {
	end := "ongoing"
	if p.ImpactEnd != nil {
		end = p.ImpactEnd.Format(time.RFC3339)
	}
	_, _ = fmt.Fprintf(w, "Postmortem %s (%s)\nimpact window %s to %s\n", safeTerminalText(p.IncidentID), safeTerminalText(p.State), p.ImpactStart.Format(time.RFC3339), end)
	for _, s := range []struct{ label, text string }{{"Summary", p.Summary}, {"Impact", p.Impact}, {"Root cause", p.RootCause}, {"Resolution", p.Resolution}} {
		if strings.TrimSpace(s.text) != "" {
			_, _ = fmt.Fprintf(w, "\n%s\n  %s\n", s.label, safeTerminalBlock(s.text))
		}
	}
	if len(p.ActionItems) > 0 {
		_, _ = fmt.Fprintln(w, "\nAction items")
		for _, it := range p.ActionItems {
			_, _ = fmt.Fprintf(w, "  - %s\n", safeTerminalText(it))
		}
	}
	if len(p.Alerts) > 0 {
		_, _ = fmt.Fprintln(w, "\nAlerts")
		for _, al := range p.Alerts {
			_, _ = fmt.Fprintf(w, "  %-10s %s\n", safeTerminalText(al.Kind), safeTerminalText(al.Ref))
		}
	}
	if p.PublishedAt != nil {
		_, _ = fmt.Fprintf(w, "\npublished %s by %s\n", p.PublishedAt.Format(time.RFC3339), safeTerminalText(p.PublishedBy))
	}
}

// safeTerminalBlock keeps line breaks of multi-line text while sanitizing each line.
func safeTerminalBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = safeTerminalText(l)
	}
	return strings.Join(lines, "\n  ")
}

// writeIncidentResult renders a JSON value or the plain printer, the shared CLI result path.
func writeIncidentResult(stdout, stderr io.Writer, of outputFlags, value any, plain func()) int {
	return writeScheduledTaskResult(stdout, stderr, of, value, plain)
}
