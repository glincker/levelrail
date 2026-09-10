package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// runStaticSites implements "static-sites list": GET /api/v1/static-sites
// (internal/api/static_sites.go's handleListStaticSites), a filtered,
// read-only view of build.type: static apps. Flat, single-verb, the same
// shape "domains certificates" uses for a different read-only view: "apps
// list" already surfaces these same apps, this exists only for a quick
// "which of my apps are plain static sites, and what domains do they
// answer on" filter without hand-filtering "apps list" output.
func runStaticSites(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, staticSitesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, staticSitesUsage(prog))
		return exitOK
	case "list":
		return runStaticSitesList(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown static-sites subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, staticSitesUsage(prog))
		return exitUsage
	}
}

func staticSitesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s static-sites list [flags]

Lists every build.type: static app served directly through the embedded
Caddy ingress, with no container involved.

Run "%[1]s static-sites <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runStaticSitesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[[]staticSiteResource]{
		cmdLabel:  "static-sites list",
		jsonUsage: "print static sites as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s static-sites list [flags]\n\nLists every static site currently served directly through embedded\nCaddy.\n\nFlags:\n", prog),
		fetch:     func(c *Client, ctx context.Context) ([]staticSiteResource, error) { return c.ListStaticSites(ctx) },
		errVerb:   "list static sites",
		print:     printStaticSitesTable,
	})
}

func printStaticSitesTable(out io.Writer, sites []staticSiteResource) {
	if len(sites) == 0 {
		_, _ = fmt.Fprintln(out, "no static sites")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tDOMAINS")
	for _, s := range sites {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", s.Name, strings.Join(s.Domains, ","))
	}
	_ = tw.Flush()
}
