package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDomainsSearchVisibility handles "domains search-visibility <app> <domain> [--hide|--show]".
func runDomainsSearchVisibility(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		_, _ = fmt.Fprint(stdout, domainsSearchVisibilityUsage(prog))
		return exitOK
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains search-visibility", "print the search visibility state as JSON to stdout and nothing else", stderr)
	var hide, show bool
	fs.BoolVar(&hide, "hide", false, "ask search engines to stay away from this domain")
	fs.BoolVar(&show, "show", false, "let search engines index this domain again (the default)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsSearchVisibilityUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains search-visibility", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	if hide && show {
		_, _ = fmt.Fprintf(stderr, "%s: choose --hide or --show, not both\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var res apiclient.DomainSearchVisibilityResource
	var err error
	if hide || show {
		res, err = client.SetDomainSearchVisibility(context.Background(), rest[0], rest[1], hide)
	} else {
		res, err = client.GetDomainSearchVisibility(context.Background(), rest[0], rest[1])
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("search visibility for domain %q: %w", rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		state := "visible to search engines"
		if res.Hidden {
			state = "hidden from search engines (noindex header and a disallow-all robots.txt)"
		}
		_, _ = fmt.Fprintf(stdout, "%s is %s\n", res.Domain, state)
	})
}

func domainsSearchVisibilityUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains search-visibility <app> <domain>           show whether a domain is hidden
  %[1]s domains search-visibility <app> <domain> --hide    ask search engines to stay away
  %[1]s domains search-visibility <app> <domain> --show    allow indexing again (default)

A hidden domain sends "X-Robots-Tag: noindex, nofollow, noarchive, nosnippet" on
every response and answers /robots.txt itself with a disallow-all file, even if
the app serves its own. This is a request that well-behaved crawlers honour; it
is not access control (use basic auth for that). Takes effect on the next
ingress reconcile pass.
`, prog)
}
