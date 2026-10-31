package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const incidentsUsage = `Usage:
  %[1]s incidents list [--state triggered|acknowledged|resolved] [--json]
  %[1]s incidents show <id> [--json]
  %[1]s incidents ack <id>
  %[1]s incidents resolve <id>
  %[1]s incidents note <id> --body <text>
  %[1]s incidents promote <alert-rule-id> [--title <text>]
  %[1]s incidents draft <id> --title <text> --body <text> [--status <s>] [--impact <i>] [--components a,b]
  %[1]s incidents publish <id> [--updated-at <ts>]
`

// runIncidents dispatches the internal incident commands. Nothing reaches the public status page
// except `publish`, which is explicit.
func runIncidents(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, incidentsUsage, prog)
		return 2
	}
	rest := args[1:]
	switch args[0] {
	case "list":
		return runIncidentsList(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return runIncidentsID(prog, "show", rest, stdout, stderr, lookupEnv, func(c *apiclient.Client, id string) (any, func(), error) {
			inc, err := c.GetIncident(context.Background(), id)
			return inc, func() { printIncident(stdout, inc) }, err
		})
	case "ack":
		return runIncidentsID(prog, "ack", rest, stdout, stderr, lookupEnv, func(c *apiclient.Client, id string) (any, func(), error) {
			st, err := c.AckIncident(context.Background(), id)
			return st, func() { _, _ = fmt.Fprintf(stdout, "%s is now %s\n", st.ID, st.InternalState) }, err
		})
	case "resolve":
		return runIncidentsID(prog, "resolve", rest, stdout, stderr, lookupEnv, func(c *apiclient.Client, id string) (any, func(), error) {
			st, err := c.ResolveIncident(context.Background(), id)
			return st, func() { _, _ = fmt.Fprintf(stdout, "%s is now %s\n", st.ID, st.InternalState) }, err
		})
	case "note":
		return runIncidentsNote(prog, rest, stdout, stderr, lookupEnv)
	case "promote":
		return runIncidentsPromote(prog, rest, stdout, stderr, lookupEnv)
	case "draft":
		return runIncidentsDraft(prog, rest, stdout, stderr, lookupEnv)
	case "publish":
		return runIncidentsPublish(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "unknown incidents command %q\n", args[0])
	return 2
}

func runIncidentsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "incidents list", "print the incidents as JSON to stdout and nothing else", stderr)
	state := fs.String("state", "", "only incidents in this internal state: triggered, acknowledged or resolved")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s incidents list [--state <triggered|acknowledged|resolved>] [flags]\n\nLists internal incidents, newest first. Needs read:sensitive.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, jsonOut, of, exitCode, ok := parseListClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, prog, stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	rows, err := client.ListIncidents(context.Background(), *state)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list incidents: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rows, func() {
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(stdout, "No incidents.")
			return
		}
		for _, i := range rows {
			pub := "internal"
			if i.Published {
				pub = "public"
			}
			_, _ = fmt.Fprintf(stdout, "%s  %-12s %-8s %-8s %s\n", i.ID, i.InternalState, i.Impact, pub, i.Title)
		}
	})
}

// runIncidentsID runs a one-argument verb; call returns the JSON value, the plain printer and the API error.
func runIncidentsID(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), call func(*apiclient.Client, string) (any, func(), error)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "incidents "+verb, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s incidents %s <id> [flags]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, stderr, singleArgCmd{prog, "incidents " + verb, "incident id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	value, plain, err := call(client, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents %s %q: %w", verb, id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, value, plain)
}

func runIncidentsNote(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "incidents note", "print the result as JSON to stdout and nothing else", stderr)
	body := fs.String("body", "", "note text (required); internal only, never shown on the public page")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s incidents note <id> --body <text> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, stderr, singleArgCmd{prog, "incidents note", "incident id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if strings.TrimSpace(*body) == "" {
		return reportError(stdout, stderr, jsonOut, errors.New("incidents note: --body is required"))
	}
	st, err := client.AddIncidentNote(context.Background(), id, *body)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents note %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, st, func() { _, _ = fmt.Fprintf(stdout, "Note added to %s\n", st.ID) })
}

func runIncidentsPromote(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "incidents promote", "print the result as JSON to stdout and nothing else", stderr)
	title := fs.String("title", "", "incident title (default \"Alert: <rule name>\")")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s incidents promote <alert-rule-id> [--title <text>] [flags]\n\nOpens an unpublished internal incident from a firing alert rule.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, rule, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, stderr, singleArgCmd{prog, "incidents promote", "alert rule id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	st, err := client.PromoteIncident(context.Background(), rule, *title)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents promote %q: %w", rule, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, st, func() { _, _ = fmt.Fprintf(stdout, "Opened %s (%s)\n", st.ID, st.InternalState) })
}

func runIncidentsDraft(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "incidents draft", "print the result as JSON to stdout and nothing else", stderr)
	title := fs.String("title", "", "public title (required)")
	body := fs.String("body", "", "public first update text (required)")
	status := fs.String("status", "", "investigating, identified, monitoring or resolved (default: keep)")
	impact := fs.String("impact", "", "none, minor, major or critical (default: keep)")
	components := fs.String("components", "", "comma separated status page component ids (default: keep; empty value clears)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s incidents draft <id> --title <text> --body <text> [--status <s>] [--impact <i>] [--components <ids>] [flags]\n\nSaves the public text. Nothing is public until `incidents publish`.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, stderr, singleArgCmd{prog, "incidents draft", "incident id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	req := apiclient.IncidentDraftRequest{Title: *title, Body: *body, Status: *status, Impact: *impact}
	fs.Visit(func(f *flag.Flag) {
		if f.Name != "components" {
			return
		}
		ids := []string{}
		for _, c := range strings.Split(*components, ",") {
			if c = strings.TrimSpace(c); c != "" {
				ids = append(ids, c)
			}
		}
		req.ComponentIDs = &ids
	})
	st, err := client.SaveIncidentDraft(context.Background(), id, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents draft %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, st, func() { _, _ = fmt.Fprintf(stdout, "Draft saved for %s (not public)\n", st.ID) })
}

func runIncidentsPublish(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "incidents publish", "print the result as JSON to stdout and nothing else", stderr)
	reviewed := fs.String("updated-at", "", "updated_at you reviewed; publishing is refused if the incident changed since (default: the value just fetched)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s incidents publish <id> [--updated-at <ts>] [flags]\n\nPublishes the saved draft to the public status page. This cannot be undone.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, stderr, singleArgCmd{prog, "incidents publish", "incident id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	inc, err := client.GetIncident(ctx, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents publish %q: load incident: %w", id, err))
	}
	if err := checkIncidentPublishable(inc, *reviewed); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents publish %q: %w", id, err))
	}
	res, err := client.PublishIncident(ctx, id, inc.UpdatedAt)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("incidents publish %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "Published %s at %s\n", res.ID, res.PublishedAt.Format(time.RFC3339))
	})
}

// checkIncidentPublishable refuses before any write: published, empty draft or a stale reviewed stamp.
func checkIncidentPublishable(inc apiclient.Incident, reviewed string) error {
	if inc.Published {
		return errors.New("already published; post a public update instead")
	}
	if inc.Draft == nil || strings.TrimSpace(inc.Draft.Title) == "" || strings.TrimSpace(inc.Draft.Body) == "" {
		return errors.New("the draft is empty; save a title and body first with `incidents draft`")
	}
	if reviewed == "" {
		return nil
	}
	ts, err := time.Parse(time.RFC3339Nano, reviewed)
	if err != nil {
		return fmt.Errorf("--updated-at %q is not an RFC 3339 timestamp", reviewed)
	}
	if !ts.Equal(inc.UpdatedAt) {
		return fmt.Errorf("the incident changed since %s (now %s); run `incidents show` and review the draft again", reviewed, inc.UpdatedAt.Format(time.RFC3339Nano))
	}
	return nil
}

func printIncident(w io.Writer, i apiclient.Incident) {
	_, _ = fmt.Fprintf(w, "%s  %s\nstate %s, status %s, impact %s, source %s %s\nupdated_at %s\n", i.ID, i.Title, i.InternalState, i.Status, i.Impact, i.Source, i.SourceRef, i.UpdatedAt.Format(time.RFC3339Nano))
	if len(i.ComponentIDs) > 0 {
		_, _ = fmt.Fprintf(w, "components %s\n", strings.Join(i.ComponentIDs, ", "))
	}
	if i.Draft != nil {
		_, _ = fmt.Fprintf(w, "\nDraft (public only after publish)\n  %s\n  %s\n", i.Draft.Title, i.Draft.Body)
	}
	if len(i.Timeline) > 0 {
		_, _ = fmt.Fprintln(w, "\nTimeline")
		for _, e := range i.Timeline {
			_, _ = fmt.Fprintf(w, "  %s  %-8s %-14s %s\n", e.CreatedAt.Format("2006-01-02 15:04"), e.Visibility, e.Status, e.Body)
		}
	}
}
