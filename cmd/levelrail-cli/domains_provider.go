package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runDomainsProvider(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsProviderUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsProviderUsage(prog))
		return exitOK
	case "get", "set", "test":
		return runDomainsProviderVerb(args[0], prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains provider verb %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsProviderUsage(prog))
		return exitUsage
	}
}

func runDomainsProviderVerb(verb, prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "domains provider " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var app, baseDomain string
	fs.StringVar(&app, "app", "", "apply to one app instead of the instance default")
	fs.StringVar(&baseDomain, "base-domain", "", "your own wildcard base domain, with provider \"custom\" (for example apps.example.com)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsProviderUsage(prog)) }

	// The provider name leads, so move it behind the flags the flag package would otherwise stop at.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && verb != "get" {
		args = append(append([]string{}, args[1:]...), args[0])
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	provider := ""
	switch verb {
	case "set":
		p, ok := requireOneArg(fs, stderr, prog, label, "provider name (sslip.io, nip.io, traefik.me, custom, or inherit with --app)")
		if !ok {
			return exitUsage
		}
		provider = p
	case "test":
		if r := fs.Args(); len(r) > 1 {
			_, _ = fmt.Fprintf(stderr, "%s: %s takes at most one provider name\n", prog, label)
			return exitUsage
		} else if len(r) == 1 {
			provider = r[0]
		}
	}
	if provider == "inherit" || provider == "default" {
		provider = ""
	}
	req := apiclient.DNSProviderRequest{Provider: provider, CustomBaseDomain: baseDomain}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	switch verb {
	case "get":
		st, err := client.GetDNSProvider(ctx, app)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get DNS provider: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, st, func() { printDNSProviderState(stdout, st, app) })
	case "set":
		if app == "" && provider == "" {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("the instance default needs a provider name"))
		}
		st, err := client.SetDNSProvider(ctx, app, req)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set DNS provider: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, st, func() { printDNSProviderState(stdout, st, app) })
	default:
		res, err := client.TestDNSProvider(ctx, app, req)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("test DNS provider: %w", err))
		}
		code := exitOK
		if res.Status != "ok" {
			code = exitCheckFailed
		}
		if rc := writeScheduledTaskResult(stdout, stderr, of, res, func() { printDNSProviderTest(stdout, res) }); rc != exitOK {
			return rc
		}
		return code
	}
}

func printDNSProviderState(out io.Writer, st apiclient.DNSProviderState, app string) {
	_, _ = fmt.Fprintf(out, "default:   %s\n", describeDNSChoice(st.Default))
	if app != "" {
		_, _ = fmt.Fprintf(out, "app:       %s\n", describeDNSChoice(st.Effective))
		if st.Hostname != "" {
			_, _ = fmt.Fprintf(out, "hostname:  %s\n", st.Hostname)
		}
	}
	_, _ = fmt.Fprintln(out, "providers:")
	for _, p := range st.Providers {
		_, _ = fmt.Fprintf(out, "  %-12s %s\n", p.ID, p.Note)
	}
	_, _ = fmt.Fprintln(out, "  custom       Your own wildcard base domain (--base-domain).")
}

func describeDNSChoice(c apiclient.DNSProviderEffective) string {
	s := c.Provider
	if c.CustomBaseDomain != "" {
		s += " (" + c.CustomBaseDomain + ")"
	}
	if c.Inherited {
		s += ", inherited"
	}
	return s
}

func printDNSProviderTest(out io.Writer, r apiclient.DNSProviderTestResult) {
	_, _ = fmt.Fprintf(out, "status:   %s\n", r.Status)
	_, _ = fmt.Fprintf(out, "provider: %s\n", r.Provider)
	if r.Hostname != "" {
		_, _ = fmt.Fprintf(out, "hostname: %s\n", r.Hostname)
	}
	if len(r.Resolved) > 0 {
		_, _ = fmt.Fprintf(out, "resolved: %s\n", strings.Join(r.Resolved, ", "))
	}
	_, _ = fmt.Fprintln(out, r.Message)
	for _, f := range r.Fixes {
		if f.Kind == "switch_provider" {
			_, _ = fmt.Fprintf(out, "fix: switch provider with 'domains provider set %s'\n", f.Provider)
		} else {
			_, _ = fmt.Fprintln(out, "fix: use your own domain with 'domains provider set custom --base-domain <domain>'")
		}
	}
	if r.DigCommand != "" {
		_, _ = fmt.Fprintf(out, "check by hand: %s\n", r.DigCommand)
	}
}

func domainsProviderUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains provider get [--app <app>]
  %[1]s domains provider set <sslip.io|nip.io|traefik.me> [--app <app>]
  %[1]s domains provider set custom --base-domain <apps.example.com> [--app <app>]
  %[1]s domains provider set inherit --app <app>     follow the instance default again
  %[1]s domains provider test [<name> [--base-domain <domain>]] [--app <app>]

Chooses the free wildcard-DNS provider behind automatic app hostnames. Changing it
re-derives the hostname with no downtime; the old hostname stops working.
`, prog)
}
