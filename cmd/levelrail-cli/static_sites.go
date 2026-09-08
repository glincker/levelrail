package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// runStaticSites dispatches "static-sites <verb> [flags]", currently
// just "list": GET /api/v1/static-sites (internal/api/static_sites.go's
// own handleListStaticSites doc comment), every build.type: static site
// this control plane serves directly through embedded Caddy, no
// container involved.
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
  %[1]s static-sites list [flags]   list every static site served directly by embedded Caddy

Run "%[1]s static-sites <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runStaticSitesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "static-sites list", "print static sites as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, staticSitesListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	sites, err := client.ListStaticSites(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list static sites: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, sites, func() { printStaticSitesTable(stdout, sites) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
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

func staticSitesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s static-sites list [flags]

Lists every static site (build.type: static in app.yaml) currently served
directly by embedded Caddy, with no container involved.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print static sites as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
