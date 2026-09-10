package main

import (
	"context"
	"fmt"
	"io"
)

// domainsWAFJSONUsage is every domains waf subcommand's --json flag
// description: identical across get/set/clear since each returns the
// same WAF/rate-limit-state shape.
const domainsWAFJSONUsage = "print the waf/rate-limit state as JSON to stdout and nothing else"

// runDomainsWAF dispatches "domains waf <verb> [flags]" to one of
// get/set/clear, mirroring runDomainsMaintenance's own top-level
// dispatch shape for a different per-domain toggle.
func runDomainsWAF(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsWAFUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsWAFUsage(prog))
		return exitOK
	case "get":
		return runDomainsWAFGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsWAFSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsWAFClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains waf subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsWAFUsage(prog))
		return exitUsage
	}
}

func domainsWAFUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains waf get <app> <domain> [flags]     show a domain's WAF and rate-limit state
  %[1]s domains waf set <app> <domain> [flags]     enable/configure the WAF and/or rate limiting
  %[1]s domains waf clear <app> <domain> [flags]   turn both off, reverting to unfiltered routing

Opt-in per-domain Web Application Firewall (OWASP Coraza, running the
stock OWASP Core Rule Set) and rate limiting, both enforced by the
embedded Caddy ingress on the next reconcile pass, no separate
container or service. --mode defaults to "detect" (CRS runs and logs
matches but never rejects a request) rather than "block": see
docs/domains-and-ingress.md for why detection-only is the recommended
first-enable default. <domain> must already be one of <app>'s
configured domains (see "%[1]s apps get <app>" or "%[1]s domains list").

Run "%[1]s domains waf <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsWAFGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains waf get", domainsWAFJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains waf get <app> <domain> [flags]\n\nShows a domain's currently configured WAF and rate-limit state.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains waf get", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	w, err := client.GetDomainWAF(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get waf state for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, w, func() { printDomainWAFHuman(stdout, w) })
}

func runDomainsWAFSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains waf set", domainsWAFJSONUsage, stderr)
	var wafFlag bool
	var modeFlag string
	var rpsFlag, burstFlag int
	fs.BoolVar(&wafFlag, "waf", false, "enable the OWASP Coraza WAF for this domain")
	fs.StringVar(&modeFlag, "mode", "detect", `WAF mode: "detect" (log only) or "block" (reject matching requests)`)
	fs.IntVar(&rpsFlag, "rps", 0, "sustained requests-per-second cap per client IP (0 disables rate limiting)")
	fs.IntVar(&burstFlag, "burst", 0, "short-window (1s) burst cap per client IP; values <= rps add no extra allowance")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains waf set <app> <domain> [flags]\n\nConfigures the WAF and/or rate limiting for <domain>. Flags not passed keep their shown defaults, they do not merge with a previously saved value: pass every setting you want to keep on every call.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains waf set", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	w, err := client.SetDomainWAF(context.Background(), appName, domain, setDomainWAFRequest{
		WAFEnabled:     wafFlag,
		WAFMode:        modeFlag,
		RateLimitRPS:   rpsFlag,
		RateLimitBurst: burstFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set waf state for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, w, func() { printDomainWAFHuman(stdout, w) })
}

func runDomainsWAFClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains waf clear", domainsWAFJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains waf clear <app> <domain> [flags]\n\nTurns off both the WAF and rate limiting for <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains waf clear", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	w, err := client.ClearDomainWAF(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear waf state for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, w, func() {
		_, _ = fmt.Fprintf(stdout, "waf and rate limiting disabled for domain %q\n", domain)
	})
}

func printDomainWAFHuman(out io.Writer, w domainWAFResource) {
	wafStatus := "disabled"
	if w.WAFEnabled {
		wafStatus = fmt.Sprintf("enabled (%s)", w.WAFMode)
	}
	rateLimitStatus := "disabled"
	if w.RateLimitEnabled {
		rateLimitStatus = fmt.Sprintf("%d req/s, burst %d", w.RateLimitRPS, w.RateLimitBurst)
	}
	_, _ = fmt.Fprintf(out, "domain:      %s\nwaf:         %s\nrate_limit:  %s\n", w.Domain, wafStatus, rateLimitStatus)
}
