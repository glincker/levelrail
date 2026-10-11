package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const domainsAuthGateJSONUsage = "print the auth gate state as JSON to stdout and nothing else"

// runDomainsAuthGate dispatches "domains auth-gate <verb> [flags]".
func runDomainsAuthGate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsAuthGateUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsAuthGateUsage(prog))
		return exitOK
	case "get":
		return runDomainsAuthGateGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsAuthGateSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsAuthGateClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains auth-gate subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsAuthGateUsage(prog))
		return exitUsage
	}
}

func domainsAuthGateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains auth-gate get <app> <domain> [flags]                 show a domain's auth gate
  %[1]s domains auth-gate set <app> <domain> --url URL [flags]       check every request with an auth service
  %[1]s domains auth-gate clear <app> <domain> [flags]               remove the auth gate

Sends every request for <domain> to your auth service first (forward auth).
A 2xx answer lets the request through with the --copy-header values set;
any other answer (for example a 302 to a login page) goes back to the
client unchanged. Client-sent copies of the copied headers are always
stripped, so they cannot be spoofed.

Run "%[1]s domains auth-gate <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsAuthGateGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains auth-gate get", domainsAuthGateJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains auth-gate get <app> <domain> [flags]\n\nShows a domain's auth gate.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains auth-gate get", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	g, err := client.GetDomainAuthGate(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get auth gate for domain %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, g, func() { printDomainAuthGateHuman(stdout, g) })
}

func runDomainsAuthGateSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains auth-gate set", domainsAuthGateJSONUsage, stderr)
	var urlFlag, checkPathFlag string
	var headers stringList
	var timeoutFlag int
	var disableFlag bool
	fs.StringVar(&urlFlag, "url", "", "auth service base URL, http or https, no credentials (required)")
	fs.StringVar(&checkPathFlag, "check-path", "/", "path on the auth service that answers the check")
	fs.Var(&headers, "copy-header", "auth response header to pass to the app on success (repeatable or comma separated, max 16)")
	fs.IntVar(&timeoutFlag, "timeout", 0, "seconds to wait for the auth service (0 uses the server default, APP_AUTH_GATE_TIMEOUT)")
	fs.BoolVar(&disableFlag, "disable", false, "save the settings but leave the gate off")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains auth-gate set <app> <domain> --url URL [flags]\n\nTurns on the auth gate for <domain>. Every call replaces the whole setting.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains auth-gate set", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	if strings.TrimSpace(urlFlag) == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains auth-gate set requires --url\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	copyHeaders := []string{}
	for _, h := range headers {
		for _, part := range strings.Split(h, ",") {
			if part = strings.TrimSpace(part); part != "" {
				copyHeaders = append(copyHeaders, part)
			}
		}
	}
	g, err := client.SetDomainAuthGate(context.Background(), appName, domain, setDomainAuthGateRequest{
		Enabled:        !disableFlag,
		AuthURL:        urlFlag,
		CheckPath:      checkPathFlag,
		CopyHeaders:    copyHeaders,
		TimeoutSeconds: timeoutFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set auth gate for domain %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, g, func() { printDomainAuthGateHuman(stdout, g) })
}

func runDomainsAuthGateClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains auth-gate clear", domainsAuthGateJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains auth-gate clear <app> <domain> [flags]\n\nRemoves the auth gate from <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains auth-gate clear", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	g, err := client.ClearDomainAuthGate(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear auth gate for domain %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, g, func() {
		_, _ = fmt.Fprintf(stdout, "auth gate removed for domain %q\n", domain)
	})
}

func printDomainAuthGateHuman(out io.Writer, g domainAuthGateResource) {
	status := "off"
	if g.Enabled {
		status = "on"
	}
	authURL := g.AuthURL
	if authURL == "" {
		authURL = "(not set)"
	}
	timeout := fmt.Sprintf("%ds (server default)", g.DefaultTimeoutSeconds)
	if g.TimeoutSeconds > 0 {
		timeout = fmt.Sprintf("%ds", g.TimeoutSeconds)
	}
	headers := strings.Join(g.CopyHeaders, ", ")
	if headers == "" {
		headers = "(none)"
	}
	_, _ = fmt.Fprintf(out, "domain:       %s\nstatus:       %s\nauth_url:     %s\ncheck_path:   %s\ncopy_headers: %s\ntimeout:      %s\n",
		g.Domain, status, authURL, g.CheckPath, headers, timeout)
}
