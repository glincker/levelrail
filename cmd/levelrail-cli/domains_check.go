package main

import (
	"context"
	"fmt"
	"io"
)

// runDomainsCheck implements "domains check <app> <domain>": GET
// /api/v1/apps/{name}/domains/{domain}/check, a real DNS lookup
// (internal/api/domain_check.go's handleCheckDomain) reporting whether
// domain currently resolves to this control plane's own advertised
// address. Read-only, unlike every other domains subcommand's nested
// get/set/clear shape: there is nothing to configure here, only a
// single check to run, so this is a direct top-level verb like "list".
func runDomainsCheck(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains check", "print the check result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsCheckUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains check", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.CheckDomain(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check domain %q for app %q: %w", domain, appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printDomainCheckHuman(stdout, result) })
}

func printDomainCheckHuman(out io.Writer, r domainCheckResource) {
	_, _ = fmt.Fprintf(out, "domain:   %s\n", r.Domain)
	_, _ = fmt.Fprintf(out, "status:   %s\n", r.Status)
	if r.ExpectedHost != "" {
		inferred := ""
		if r.HostInferred {
			inferred = " (best guess, configure APP_PUBLIC_HOST for accuracy)"
		}
		_, _ = fmt.Fprintf(out, "expected: %s%s\n", r.ExpectedHost, inferred)
	}
	if len(r.ExpectedIPv4) > 0 {
		_, _ = fmt.Fprintf(out, "expected ipv4: %v\n", r.ExpectedIPv4)
	}
	if len(r.ExpectedIPv6) > 0 {
		_, _ = fmt.Fprintf(out, "expected ipv6: %v\n", r.ExpectedIPv6)
	}
	if len(r.ResolvedHosts) > 0 {
		_, _ = fmt.Fprintf(out, "resolved: %v\n", r.ResolvedHosts)
	}
}

func domainsCheckUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains check <app> <domain> [flags]

Runs a real DNS lookup for <domain> and reports whether it currently
resolves to this control plane's own advertised address: one of
"connected", "not_resolving", "resolves_elsewhere", or "unconfigured"
(no APP_PUBLIC_HOST and no usable request host to infer one from).
<domain> must already be one of <app>'s configured domains.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the check result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
