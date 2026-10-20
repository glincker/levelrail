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
	case "log":
		return runDomainsAuthGateLog(prog, args[1:], stdout, stderr, lookupEnv)
	case "revoke":
		return runDomainsAuthGateRevoke(prog, args[1:], stdout, stderr, lookupEnv)
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
  %[1]s domains auth-gate set <app> <domain> --provider builtin --allow EMAIL [flags]
                                                                    require this installation's sign-in
  %[1]s domains auth-gate clear <app> <domain> [flags]               remove the auth gate
  %[1]s domains auth-gate log <app> <domain> [--since 24h]           show the built-in sign-in access log
  %[1]s domains auth-gate revoke <app> <domain> [flags]              sign out every visitor of the domain

Sends every request for <domain> to an auth check first (forward auth).
With --provider external (the default) your auth service answers: a 2xx
lets the request through with the --copy-header values set, anything else
(for example a 302 to a login page) goes back to the client unchanged.

With --provider builtin visitors sign in with their account on this
installation and only the users, teams and roles you allow get through;
the app receives X-User, X-Email and X-Name. With no allow rule nobody
gets in. Client-sent copies of the copied headers are always stripped.

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

func splitCommaList(values []string) []string {
	out := []string{}
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func runDomainsAuthGateSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains auth-gate set", domainsAuthGateJSONUsage, stderr)
	var urlFlag, checkPathFlag, providerFlag string
	var headers, allowUsers, allowTeams, allowRoles stringList
	var timeoutFlag int
	var disableFlag bool
	fs.StringVar(&providerFlag, "provider", "external", "who checks visitors: external (your auth service) or builtin (this installation's sign-in)")
	fs.StringVar(&urlFlag, "url", "", "auth service base URL, http or https, no credentials (required with --provider external)")
	fs.StringVar(&checkPathFlag, "check-path", "/", "path on the auth service that answers the check")
	fs.Var(&headers, "copy-header", "auth response header to pass to the app on success (repeatable or comma separated, max 16)")
	fs.Var(&allowUsers, "allow", "builtin: let this user in, by email (repeatable or comma separated)")
	fs.Var(&allowTeams, "allow-team", "builtin: let members of this team in, by name or id (repeatable)")
	fs.Var(&allowRoles, "allow-role", "builtin: let users with this role in, by name or id (repeatable)")
	fs.IntVar(&timeoutFlag, "timeout", 0, "seconds to wait for the auth check (0 uses the server default, APP_AUTH_GATE_TIMEOUT)")
	fs.BoolVar(&disableFlag, "disable", false, "save the settings but leave the gate off")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains auth-gate set <app> <domain> [--url URL | --provider builtin --allow EMAIL] [flags]\n\nTurns on the auth gate for <domain>. Every call replaces the whole setting, allow rules included.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains auth-gate set", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	var allow []authGateRule
	for _, kind := range []struct {
		name   string
		values []string
	}{{"user", allowUsers}, {"team", allowTeams}, {"role", allowRoles}} {
		for _, v := range splitCommaList(kind.values) {
			allow = append(allow, authGateRule{Kind: kind.name, Subject: v})
		}
	}
	builtin := !strings.EqualFold(strings.TrimSpace(providerFlag), "external")
	if !builtin && strings.TrimSpace(urlFlag) == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains auth-gate set requires --url (or --provider builtin)\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if !builtin && len(allow) > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: --allow, --allow-team and --allow-role need --provider builtin\n\n", prog)
		return exitUsage
	}
	if builtin && len(allow) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: warning: no --allow rule, so nobody can get in until you add one\n", prog)
	}
	g, err := client.SetDomainAuthGate(context.Background(), appName, domain, setDomainAuthGateRequest{
		Enabled:        !disableFlag,
		Provider:       providerFlag,
		AuthURL:        urlFlag,
		CheckPath:      checkPathFlag,
		CopyHeaders:    splitCommaList(headers),
		TimeoutSeconds: timeoutFlag,
		Allow:          allow,
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

func runDomainsAuthGateLog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains auth-gate log", "print the access log as JSON to stdout and nothing else", stderr)
	var sinceFlag string
	fs.StringVar(&sinceFlag, "since", "24h", "how far back to look, a duration such as 1h, 24h or 7d")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains auth-gate log <app> <domain> [--since 24h] [flags]\n\nShows who passed the built-in sign-in gate, who was turned away and why. Repeated allows are counted per user and minute.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains auth-gate log", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.ListAuthGateEvents(context.Background(), appName, domain, sinceFlag)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("auth gate log for domain %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printAuthGateLogHuman(stdout, res) })
}

func runDomainsAuthGateRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains auth-gate revoke", domainsAuthGateJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains auth-gate revoke <app> <domain> [flags]\n\nSigns out every visitor of <domain> who signed in through the built-in provider.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains auth-gate revoke", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.RevokeAuthGateSessions(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke auth gate sessions for domain %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "signed out %d visitor session(s) on %q\n", res.Revoked, domain)
	})
}

func printAuthGateLogHuman(out io.Writer, res authGateEventsResponse) {
	if len(res.Events) == 0 {
		_, _ = fmt.Fprintf(out, "no auth gate activity on %s since %s\n", res.Domain, res.Since)
		return
	}
	_, _ = fmt.Fprintf(out, "%-27s  %-30s  %-16s  %-24s  %-15s  %s\n", "TIME", "USER", "DECISION", "REASON", "IP", "COUNT")
	for _, e := range res.Events {
		user := e.UserEmail
		if user == "" {
			user = e.UserID
		}
		if user == "" {
			user = "-"
		}
		_, _ = fmt.Fprintf(out, "%-27s  %-30s  %-16s  %-24s  %-15s  %d\n", e.At, user, e.Decision, e.Reason, e.IP, e.Count)
	}
}

func printDomainAuthGateHuman(out io.Writer, g domainAuthGateResource) {
	status := "off"
	if g.Enabled {
		status = "on"
	}
	provider := g.Provider
	if provider == "" {
		provider = "external"
	}
	timeout := fmt.Sprintf("%ds (server default)", g.DefaultTimeoutSeconds)
	if g.TimeoutSeconds > 0 {
		timeout = fmt.Sprintf("%ds", g.TimeoutSeconds)
	}
	headers := strings.Join(g.CopyHeaders, ", ")
	if headers == "" {
		headers = "(none)"
	}
	_, _ = fmt.Fprintf(out, "domain:       %s\nstatus:       %s\nprovider:     %s\n", g.Domain, status, provider)
	if provider == "builtin" {
		allow := make([]string, 0, len(g.Allow))
		for _, r := range g.Allow {
			label := r.Label
			if label == "" {
				label = r.Subject
			}
			allow = append(allow, r.Kind+":"+label)
		}
		allowed := strings.Join(allow, ", ")
		if allowed == "" {
			allowed = "(nobody, add --allow)"
		}
		_, _ = fmt.Fprintf(out, "allow:        %s\nsessions:     %d signed in\n", allowed, g.LiveSessions)
	} else {
		authURL := g.AuthURL
		if authURL == "" {
			authURL = "(not set)"
		}
		_, _ = fmt.Fprintf(out, "auth_url:     %s\ncheck_path:   %s\n", authURL, g.CheckPath)
	}
	_, _ = fmt.Fprintf(out, "copy_headers: %s\ntimeout:      %s\n", headers, timeout)
}
