package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func searchUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s search <query...> [--kind K] [--limit N] [flags]   search apps, databases, nodes, domains, templates, settings and actions you may use
  %[1]s search saved list [flags]                          your saved searches
  %[1]s search saved add <name> <query...> [flags]         save a query under a name
  %[1]s search saved rm <id> [flags]                       delete a saved search

Query scopes: app:NAME env:NAME tag:NAME node:NAME kind:KIND, then free text.
Kinds: app, database, node, domain, template, project, environment, tag, deployment, setting, doc, action.
Example: %[1]s search kind:action restart api
`, prog)
}

func runSearch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, searchUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, searchUsage(prog))
		return exitOK
	case "saved":
		return runSearchSaved(prog, args[1:], stdout, stderr, lookupEnv)
	}
	return runSearchQuery(prog, args, stdout, stderr, lookupEnv)
}

func runSearchQuery(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "search", "print the hits as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, searchUsage(prog)) }
	kind := fs.String("kind", "", "limit results to one kind (app, database, action, ...)")
	limit := fs.Int("limit", 20, "maximum hits to return")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, flagsFirst(args), apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	q := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(q) == "" {
		_, _ = fmt.Fprint(stderr, searchUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	resp, err := client.Search(context.Background(), q, *kind, *limit, "")
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("search: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, resp, func() { printSearchHits(stdout, resp) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSearchHits(out io.Writer, resp apiclient.SearchResponse) {
	if len(resp.Hits) == 0 {
		_, _ = fmt.Fprintln(out, "no matches")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tTITLE\tDETAIL")
	for _, h := range resp.Hits {
		detail := h.Subtitle
		if h.Action != nil {
			detail = "needs " + h.Action.Ability
			if h.Action.Confirm {
				detail += ", asks to confirm"
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", h.Kind, h.Title, detail)
	}
	_ = tw.Flush()
	if resp.NextCursor != "" {
		_, _ = fmt.Fprintf(out, "%d more, raise --limit to see them\n", resp.Total-len(resp.Hits))
	}
}

func runSearchSaved(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, searchUsage(prog))
		return exitUsage
	}
	sub := args[0]
	if sub != "list" && sub != "add" && sub != "rm" {
		_, _ = fmt.Fprintf(stderr, "%s: unknown search saved subcommand %q\n\n%s", prog, sub, searchUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "search saved "+sub, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, searchUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, flagsFirst(args[1:]), apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch sub {
	case "list":
		saved, err := client.ListSavedSearches(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("search saved list: %w", err))
		}
		return renderSaved(stdout, stderr, of, saved, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tNAME\tQUERY")
			for _, s := range saved {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ID, s.Name, s.Query)
			}
			_ = tw.Flush()
		})
	case "add":
		if len(rest) < 2 {
			_, _ = fmt.Fprint(stderr, searchUsage(prog))
			return exitUsage
		}
		s, err := client.SaveSearch(ctx, rest[0], strings.Join(rest[1:], " "))
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("search saved add: %w", err))
		}
		return renderSaved(stdout, stderr, of, s, func() { _, _ = fmt.Fprintf(stdout, "saved %q as %s\n", s.Name, s.ID) })
	}
	if len(rest) != 1 {
		_, _ = fmt.Fprint(stderr, searchUsage(prog))
		return exitUsage
	}
	if err := client.DeleteSavedSearch(ctx, rest[0]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("search saved rm: %w", err))
	}
	return renderSaved(stdout, stderr, of, map[string]string{"deleted": rest[0]}, func() { _, _ = fmt.Fprintf(stdout, "deleted %s\n", rest[0]) })
}

func renderSaved(stdout, stderr io.Writer, of outputFlags, data any, table func()) int {
	if err := renderResult(stdout, of.Format, of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

var searchValueFlags = map[string]bool{"kind": true, "limit": true, "token": true, "api-url": true, "profile": true, "output": true, "query": true}

// flagsFirst moves flags ahead of the positional query so `search restart api --json` works.
func flagsFirst(args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := strings.TrimLeft(a, "-")
		if !strings.HasPrefix(a, "-") || a == "-" || name == "" {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		if !strings.Contains(name, "=") && searchValueFlags[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, rest...)
}
