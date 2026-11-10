package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func viewsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s views list [--list L] [flags]                                    your saved views and the views shared with your team
  %[1]s views save <name> --list L [--filter KEY=VALUE]... [--share private|team] [flags]
                                                                         save a list filter under a name (same name replaces it)
  %[1]s views delete <id|name> [--list L] [flags]                        delete a view (owner, or a team admin for a shared view)
  %[1]s views apply <id|name> [--list L] [flags]                         print the dashboard path that opens the list with the view's filter

Lists and their filter keys:
  apps: q, status, env, tags, labels
  databases: q, status, engine, labels
  domains: q, status
  builds: q, status, app
  deployments: q, status, app, env
--share team shares the view with the team you are signed in to; only you can change it.
Example: %[1]s views save failing --list apps --filter status=failing --share team
`, prog)
}

var viewsSubcommands = map[string]bool{"list": true, "save": true, "delete": true, "apply": true}

func runViews(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, viewsUsage(prog))
		return exitUsage
	}
	sub := args[0]
	switch {
	case sub == "-h" || sub == "--help" || sub == "help":
		_, _ = fmt.Fprint(stdout, viewsUsage(prog))
		return exitOK
	case !viewsSubcommands[sub]:
		_, _ = fmt.Fprintf(stderr, "%s: unknown views subcommand %q\n\n%s", prog, sub, viewsUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "views "+sub, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, viewsUsage(prog)) }
	list := fs.String("list", "", "the list page: apps, databases, domains, builds or deployments")
	share := fs.String("share", "", "private (default) or team")
	var filters stringListFlag
	fs.Var(&filters, "filter", "a filter field as KEY=VALUE, repeatable")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, viewsFlagsFirst(args[1:]), apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if (sub == "list" && len(rest) != 0) || (sub != "list" && len(rest) != 1) {
		_, _ = fmt.Fprint(stderr, viewsUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch sub {
	case "list":
		views, err := client.ListSavedViews(ctx, *list)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("views list: %w", err))
		}
		return renderSaved(stdout, stderr, of, views, func() { printViews(stdout, views) })
	case "save":
		if *list == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--list is required"))
		}
		filter, err := parseViewFilters(filters)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		v, err := client.SaveView(ctx, apiclient.SavedViewInput{Name: rest[0], List: *list, Filter: filter, Shared: *share})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("views save: %w", err))
		}
		return renderSaved(stdout, stderr, of, v, func() {
			_, _ = fmt.Fprintf(stdout, "saved %q for %s as %s (%s)\n", safeTerminalText(v.Name), v.List, v.ID, v.Shared)
		})
	}
	v, err := findView(ctx, client, rest[0], *list)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("views %s: %w", sub, err))
	}
	switch sub {
	case "apply":
		path := viewPath(v)
		return renderSaved(stdout, stderr, of, map[string]any{"id": v.ID, "list": v.List, "filter": v.Filter, "path": path}, func() {
			_, _ = fmt.Fprintln(stdout, path)
		})
	case "delete":
		if err := client.DeleteSavedView(ctx, v.ID); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("views delete: %w", err))
		}
	}
	return renderSaved(stdout, stderr, of, map[string]string{"deleted": v.ID}, func() { _, _ = fmt.Fprintf(stdout, "deleted %s\n", v.ID) })
}

// viewsFlagsFirst moves flags ahead of the name so `views save prod --list apps` parses.
func viewsFlagsFirst(args []string) []string {
	valued := map[string]bool{"list": true, "share": true, "filter": true, "token": true, "api-url": true, "profile": true, "output": true, "query": true}
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := strings.TrimLeft(a, "-")
		if !strings.HasPrefix(a, "-") || a == "-" || name == "" {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		if !strings.Contains(name, "=") && valued[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, rest...)
}

func parseViewFilters(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, newValidationError("--filter %q must be KEY=VALUE", p)
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, nil
}

// findView matches an id first, then a name, which must be unique unless --list narrows it.
func findView(ctx context.Context, client *apiclient.Client, ref, list string) (apiclient.SavedView, error) {
	views, err := client.ListSavedViews(ctx, list)
	if err != nil {
		return apiclient.SavedView{}, err
	}
	var byName []apiclient.SavedView
	for _, v := range views {
		if v.ID == ref {
			return v, nil
		}
		if v.Name == ref && v.Mine {
			byName = append(byName, v)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		return apiclient.SavedView{}, newValidationError("no saved view %q; run views list", ref)
	}
	return apiclient.SavedView{}, newValidationError("%d views are named %q; pass --list or the id", len(byName), ref)
}

// viewPath is the dashboard list path; keys are sorted so the output is stable.
func viewPath(v apiclient.SavedView) string {
	q := url.Values{}
	keys := make([]string, 0, len(v.Filter))
	for k := range v.Filter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		q.Set(k, v.Filter[k])
	}
	path := "/" + url.PathEscape(v.List)
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return path
}

func printViews(out io.Writer, views []apiclient.SavedView) {
	if len(views) == 0 {
		_, _ = fmt.Fprintln(out, "no saved views")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tLIST\tNAME\tSHARED\tOWNER\tFILTER")
	for _, v := range views {
		owner := "you"
		if !v.Mine {
			owner = "team"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", v.ID, v.List, safeTerminalText(v.Name), v.Shared, owner, safeTerminalText(viewPath(v)))
	}
	_ = tw.Flush()
}
