package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// domainsErrorPagesJSONUsage is every domains error-pages subcommand's
// --json flag description: identical across get/set/clear since each
// returns the same error-pages-state shape.
const domainsErrorPagesJSONUsage = "print the error-pages state as JSON to stdout and nothing else"

// domainsErrorPagesAllowedCodes documents the fixed set of status codes
// this feature covers, shown in usage text and validated client-side
// before ever reaching the API.
const domainsErrorPagesAllowedCodes = "404, 500, 502, or 503"

// runDomainsErrorPages dispatches "domains error-pages <verb> [flags]"
// to one of get/set/clear, mirroring runDomainsWAF's own top-level
// dispatch shape for a different per-domain, multi-entry toggle.
func runDomainsErrorPages(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsErrorPagesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsErrorPagesUsage(prog))
		return exitOK
	case "get":
		return runDomainsErrorPagesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsErrorPagesSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsErrorPagesClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains error-pages subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsErrorPagesUsage(prog))
		return exitUsage
	}
}

func domainsErrorPagesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains error-pages get <app> <domain> [--code N] [flags]     show a domain's custom error pages
  %[1]s domains error-pages set <app> <domain> --code N (--body TEXT | --body-file PATH) [flags]
                                                                       set the custom page served for status code N
  %[1]s domains error-pages clear <app> <domain> [--code N] [flags]   remove one mapping, or all if --code is omitted

Replaces Caddy's bare default error text, or whatever the backend itself
returned, with your own HTML for one of a fixed set of status codes
(%[2]s). Enforced by the embedded Caddy ingress on the next reconcile
pass. <domain> must already be one of <app>'s configured domains (see
"%[1]s apps get <app>" or "%[1]s domains list").

Run "%[1]s domains error-pages <subcommand> -h" for a subcommand's own flags.
`, prog, domainsErrorPagesAllowedCodes)
}

func runDomainsErrorPagesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains error-pages get", domainsErrorPagesJSONUsage, stderr)
	var codeFlag int
	fs.IntVar(&codeFlag, "code", 0, "show only this status code's mapping (default: show every configured mapping)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains error-pages get <app> <domain> [--code N] [flags]\n\nShows a domain's currently configured custom error pages.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains error-pages get", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	pages, err := client.GetDomainErrorPages(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get error pages for domain %q: %w", domain, err))
	}
	if codeFlag != 0 {
		pages.Pages = filterDomainErrorPages(pages.Pages, codeFlag)
	}

	return writeScheduledTaskResult(stdout, stderr, of, pages, func() { printDomainErrorPagesHuman(stdout, pages) })
}

func runDomainsErrorPagesSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains error-pages set", domainsErrorPagesJSONUsage, stderr)
	var codeFlag int
	var bodyFlag, bodyFileFlag string
	fs.IntVar(&codeFlag, "code", 0, "status code to set a custom page for ("+domainsErrorPagesAllowedCodes+") (required)")
	fs.StringVar(&bodyFlag, "body", "", "custom HTML to serve, given inline")
	fs.StringVar(&bodyFileFlag, "body-file", "", "path to an HTML file to serve; mutually exclusive with --body")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains error-pages set <app> <domain> --code N (--body TEXT | --body-file PATH) [flags]\n\nSets the custom page <domain> serves for status code --code, replacing any previously set page for that same code.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains error-pages set", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	if codeFlag == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: --code is required\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if bodyFlag != "" && bodyFileFlag != "" {
		_, _ = fmt.Fprintf(stderr, "%s: --body and --body-file are mutually exclusive\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	body := bodyFlag
	if bodyFileFlag != "" {
		data, err := os.ReadFile(bodyFileFlag) //nolint:gosec // operator-supplied local path, the CLI's own documented input
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read --body-file %q: %w", bodyFileFlag, err))
		}
		body = string(data)
	}
	if body == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --body or --body-file is required\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	pages, err := client.SetDomainErrorPage(context.Background(), appName, domain, setDomainErrorPageRequest{
		StatusCode: codeFlag,
		Body:       body,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set error page for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, pages, func() { printDomainErrorPagesHuman(stdout, pages) })
}

func runDomainsErrorPagesClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains error-pages clear", domainsErrorPagesJSONUsage, stderr)
	var codeFlag int
	fs.IntVar(&codeFlag, "code", 0, "clear only this status code's mapping (default: clear every mapping)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains error-pages clear <app> <domain> [--code N] [flags]\n\nRemoves one mapping (--code) or every custom error page (no --code) configured on <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains error-pages clear", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	var pages domainErrorPagesResource
	var err error
	if codeFlag != 0 {
		pages, err = client.ClearDomainErrorPage(context.Background(), appName, domain, codeFlag)
	} else {
		pages, err = client.ClearDomainErrorPages(context.Background(), appName, domain)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear error pages for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, pages, func() {
		_, _ = fmt.Fprintf(stdout, "error pages cleared for domain %q\n", domain)
	})
}

// filterDomainErrorPages returns only the entries matching statusCode,
// for "domains error-pages get --code N" to narrow a full list without
// a separate, single-entry API call.
func filterDomainErrorPages(pages []domainErrorPageEntry, statusCode int) []domainErrorPageEntry {
	var out []domainErrorPageEntry
	for _, p := range pages {
		if p.StatusCode == statusCode {
			out = append(out, p)
		}
	}
	return out
}

func printDomainErrorPagesHuman(out io.Writer, r domainErrorPagesResource) {
	if len(r.Pages) == 0 {
		_, _ = fmt.Fprintf(out, "domain: %s\nstatus: no custom error pages configured\n", r.Domain)
		return
	}
	_, _ = fmt.Fprintf(out, "domain: %s\n", r.Domain)
	for _, p := range r.Pages {
		_, _ = fmt.Fprintf(out, "  %d: %d bytes\n", p.StatusCode, len(p.Body))
	}
}
