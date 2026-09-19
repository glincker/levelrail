package main

import (
	"context"
	"fmt"
	"io"
)

// domainsRedirectJSONUsage is every domains redirect subcommand's --json
// flag description: identical across get/set/clear since each returns
// the same redirect-state shape.
const domainsRedirectJSONUsage = "print the redirect state as JSON to stdout and nothing else"

// runDomainsRedirect dispatches "domains redirect <verb> [flags]" to one
// of get/set/clear, mirroring runDomainsMaintenance's own top-level
// dispatch shape for a different per-domain toggle.
func runDomainsRedirect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsRedirectUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsRedirectUsage(prog))
		return exitOK
	case "get":
		return runDomainsRedirectGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsRedirectSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsRedirectClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains redirect subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsRedirectUsage(prog))
		return exitUsage
	}
}

func domainsRedirectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains redirect get <app> <domain> [flags]     show a domain's redirect state
  %[1]s domains redirect set <app> <domain> --target URL [flags]   point <domain> at a target URL
  %[1]s domains redirect clear <app> <domain> [flags]   remove the redirect

While configured, the embedded Caddy ingress redirects every request for
<domain> to --target instead of proxying to its container, without
stopping the container itself. If <domain> also has maintenance mode
enabled, maintenance mode takes precedence and the redirect does not
apply. <domain> must already be one of <app>'s configured domains (see
"%[1]s apps get <app>" or "%[1]s domains list").

Run "%[1]s domains redirect <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsRedirectGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains redirect get", domainsRedirectJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains redirect get <app> <domain> [flags]\n\nShows a domain's currently configured redirect.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains redirect get", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	rd, err := client.GetDomainRedirect(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get redirect state for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, rd, func() { printDomainRedirectHuman(stdout, rd) })
}

func runDomainsRedirectSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains redirect set", domainsRedirectJSONUsage, stderr)
	var targetFlag string
	var permanentFlag, temporaryFlag bool
	fs.StringVar(&targetFlag, "target", "", "absolute target URL to redirect to, e.g. https://example.com (required)")
	fs.BoolVar(&permanentFlag, "permanent", false, "use a 301 permanent redirect (the default even without this flag)")
	fs.BoolVar(&temporaryFlag, "temporary", false, "use a 302 temporary redirect instead of the 301 default")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains redirect set <app> <domain> --target URL [flags]\n\nConfigures <domain> to redirect to --target. 301 permanent is the default; pass --temporary for a 302. --permanent and --temporary are mutually exclusive.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains redirect set", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	if targetFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --target is required\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if permanentFlag && temporaryFlag {
		_, _ = fmt.Fprintf(stderr, "%s: --permanent and --temporary are mutually exclusive\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	statusCode := 301
	if temporaryFlag {
		statusCode = 302
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	rd, err := client.SetDomainRedirect(context.Background(), appName, domain, setDomainRedirectRequest{
		TargetURL:  targetFlag,
		StatusCode: statusCode,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set redirect for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, rd, func() { printDomainRedirectHuman(stdout, rd) })
}

func runDomainsRedirectClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains redirect clear", domainsRedirectJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains redirect clear <app> <domain> [flags]\n\nRemoves the redirect configured on <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains redirect clear", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	rd, err := client.ClearDomainRedirect(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear redirect for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, rd, func() {
		_, _ = fmt.Fprintf(stdout, "redirect removed for domain %q\n", domain)
	})
}

func printDomainRedirectHuman(out io.Writer, rd domainRedirectResource) {
	if !rd.Enabled {
		_, _ = fmt.Fprintf(out, "domain: %s\nstatus: no redirect configured\n", rd.Domain)
		return
	}
	_, _ = fmt.Fprintf(out, "domain:      %s\ntarget:      %s\nstatus_code: %d\n", rd.Domain, rd.TargetURL, rd.StatusCode)
}
