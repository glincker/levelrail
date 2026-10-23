package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

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
	case "delete":
		return runStaticSitesDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "routes":
		return runStaticRoutes(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown static-sites subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, staticSitesUsage(prog))
		return exitUsage
	}
}

func staticSitesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s static-sites list [flags]
  %[1]s static-sites delete <name> [flags]
  %[1]s static-sites routes show|test|lint <app> [flags]

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
