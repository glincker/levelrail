package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runResources dispatches "resources <verb>": the cross-kind explorer, tags and saved views.
func runResources(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, resourcesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, resourcesUsage(prog))
		return exitOK
	case "list":
		return runResourcesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "tag":
		return runResourcesTag(prog, "add", args[1:], stdout, stderr, lookupEnv)
	case "untag":
		return runResourcesTag(prog, "remove", args[1:], stdout, stderr, lookupEnv)
	case "views":
		return runResourcesViews(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown resources subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, resourcesUsage(prog))
		return exitUsage
	}
}

func resourcesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s resources list [--q TEXT] [--kind app|database|node|domain] [--tag T]...
  %[1]s resources tag --tag T [--dry-run] KIND:NAME...      tag many resources, preview first with --dry-run
  %[1]s resources untag --tag T [--dry-run] KIND:NAME...
  %[1]s resources views [list]                              saved explorer views
  %[1]s resources views save NAME [--q TEXT] [--kind K] [--tag T]...
  %[1]s resources views delete ID

Tags are key or key:value (for example team:shop). "usage report --by tag"
groups cost by tag.
`, prog)
}

func runResourcesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "resources list", "print the list as JSON to stdout and nothing else", stderr)
	text := fs.String("q", "", "search text (name, detail, tag)")
	kind := fs.String("kind", "", "app, database, node or domain")
	limit := fs.Int("limit", 0, "most rows to return (default all)")
	var tags stringList
	fs.Var(&tags, "tag", "require this tag, repeatable (all must match)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListConsoleResources(context.Background(), apiclient.ConsoleQuery{Text: *text, Kind: *kind, Tags: tags, Limit: *limit})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list resources: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printResources(stdout, list) })
}

func printResources(out io.Writer, list apiclient.ConsoleList) {
	if len(list.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no resources match")
		return
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KIND\tNAME\tDETAIL\tTAGS")
	for _, r := range list.Items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Kind, r.Name, r.Detail, strings.Join(r.Tags, ","))
	}
	_ = w.Flush()
	_, _ = fmt.Fprintf(out, "\n%d of %d shown\n", len(list.Items), list.Total)
}

func parseResourceRefs(args []string) ([]apiclient.ConsoleRef, error) {
	refs := make([]apiclient.ConsoleRef, 0, len(args))
	for _, a := range args {
		kind, name, ok := strings.Cut(a, ":")
		if !ok || kind == "" || name == "" {
			return nil, fmt.Errorf("%q is not KIND:NAME", a)
		}
		refs = append(refs, apiclient.ConsoleRef{Kind: kind, Name: name})
	}
	return refs, nil
}

func runResourcesTag(prog, action string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "resources tag"
	if action == "remove" {
		label = "resources untag"
	}
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	tag := fs.String("tag", "", "tag to apply, key or key:value")
	dry := fs.Bool("dry-run", false, "show what would change and write nothing")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if *tag == "" || fs.NArg() == 0 {
		_, _ = fmt.Fprint(stderr, resourcesUsage(prog))
		return exitUsage
	}
	refs, err := parseResourceRefs(fs.Args())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	resp, err := client.BulkTagResources(context.Background(), apiclient.ConsoleBulkTagRequest{Action: action, Tag: *tag, Resources: refs, DryRun: *dry})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, resp, func() { printBulkTag(stdout, resp) })
}

func printBulkTag(out io.Writer, resp apiclient.ConsoleBulkTagResponse) {
	if resp.DryRun {
		_, _ = fmt.Fprintln(out, "dry run, nothing was changed")
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, r := range resp.Results {
		_, _ = fmt.Fprintf(w, "%s:%s\t%s\t%s\n", r.Kind, r.Name, r.Status, r.Message)
	}
	_ = w.Flush()
	keys := make([]string, 0, len(resp.Counts))
	for k := range resp.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, resp.Counts[k]))
	}
	_, _ = fmt.Fprintf(out, "%s\n", strings.Join(parts, ", "))
}

func runResourcesViews(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "list"
	if len(args) > 0 && (args[0] == "list" || args[0] == "save" || args[0] == "delete") {
		verb, args = args[0], args[1:]
	}
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "resources views", "print the result as JSON to stdout and nothing else", stderr)
	text := fs.String("q", "", "search text to store (save)")
	kind := fs.String("kind", "", "kind filter to store (save)")
	var tags stringList
	fs.Var(&tags, "tag", "tag filter to store, repeatable (save)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch verb {
	case "save":
		if fs.NArg() != 1 {
			_, _ = fmt.Fprint(stderr, resourcesUsage(prog))
			return exitUsage
		}
		q, _ := json.Marshal(map[string]any{"q": *text, "kind": *kind, "tags": []string(tags)})
		v, err := client.SaveConsoleView(ctx, fs.Arg(0), string(q))
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("save view: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, v, func() { _, _ = fmt.Fprintf(stdout, "saved view %s (%s)\n", v.Name, v.ID) })
	case "delete":
		if fs.NArg() != 1 {
			_, _ = fmt.Fprint(stderr, resourcesUsage(prog))
			return exitUsage
		}
		if err := client.DeleteConsoleView(ctx, fs.Arg(0)); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete view: %w", err))
		}
		_, _ = fmt.Fprintln(stdout, "deleted")
		return exitOK
	}
	views, err := client.ListConsoleViews(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list views: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, views, func() {
		if len(views) == 0 {
			_, _ = fmt.Fprintln(stdout, "no saved views")
			return
		}
		w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tNAME\tQUERY")
		for _, v := range views {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", v.ID, v.Name, v.Query)
		}
		_ = w.Flush()
	})
}
