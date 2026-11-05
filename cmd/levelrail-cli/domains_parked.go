package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func domainsParkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains park <app> <domain> [--env ENV] [--note TEXT]
  %[1]s domains park --static-site SITE <domain> [--note TEXT]
  %[1]s domains move <domain> --from APP --to APP [--from-env ENV] [--to-env ENV] [--confirm]
  %[1]s domains move <domain> [--from APP | --from-static-site SITE] [--to APP | --to-static-site SITE] [--confirm]
  %[1]s domains parked list
  %[1]s domains parked attach <domain> --app APP [--env ENV] [--discard-settings]
  %[1]s domains parked attach <domain> --static-site SITE
  %[1]s domains parked delete <domain>
  %[1]s domains parked retention

park takes a domain off an app but keeps it registered, with its DNS records
and certificate untouched, so it can be attached to any app later. move hands
a domain to another app with its per-domain settings; without --confirm it
only runs the collision check. parked delete forgets the registration and
never touches DNS. park keeps the domain's settings (WAF, rate limit, TLS
certificate reference, basic auth, auth gate, maintenance, redirect, error
pages, search settings); parked retention prints the dry run of the
automatic cleanup (setting domains.parked_retention_days, 0 = off); attach restores them unless --discard-settings.
Static sites work the same way through --static-site, --from-static-site and
--to-static-site. A static site has no per-domain settings, so a domain that
moves to or from one, or is attached to one, does not keep them.
Without --env, --from-env and --to-env a command works on the app's default
domain list; with them it works on that environment's own domain set (an
environment ID, name or kind), so an environment domain can be parked, moved
between environments or attached to another app's environment.
`, prog)
}

// runDomainsPark handles "domains park <app> <domain>".
func runDomainsPark(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains park", "print the result as JSON", stderr)
	note := fs.String("note", "", "why the domain is parked")
	env := fs.String("env", "", "environment whose domain set the domain leaves (default: the app's default list)")
	site := fs.String("static-site", "", "park the domain from this static site instead of an app")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsParkUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	req := apiclient.DomainLifecycleRequest{Environment: *env, Note: *note}
	if *site != "" {
		rest, ok := requireArgs(fs, stderr, prog, "domains park", "a domain (with --static-site)", 1)
		if !ok {
			return exitUsage
		}
		req.App, req.Domain, req.TargetKind = *site, rest[0], "static_site"
	} else {
		rest, ok := requireArgs(fs, stderr, prog, "domains park", "an app and a domain", 2)
		if !ok {
			return exitUsage
		}
		req.App, req.Domain = rest[0], rest[1]
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	res, err := client.ParkDomain(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("park domain %q: %w", req.Domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "parked %s (was on %s); attach it to an app with: %s domains parked attach %s --app APP\n", res.Domain, res.From, prog, res.Domain)
	})
}

// runDomainsMove handles "domains move <domain> --from A --to B".
func runDomainsMove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains move", "print the result as JSON", stderr)
	from := fs.String("from", "", "app that serves the domain now")
	to := fs.String("to", "", "app that should serve it")
	fromEnv := fs.String("from-env", "", "environment set the domain leaves (default: the app's default list)")
	toEnv := fs.String("to-env", "", "environment set the domain joins (default: the app's default list)")
	fromSite := fs.String("from-static-site", "", "static site that serves the domain now (instead of --from)")
	toSite := fs.String("to-static-site", "", "static site that should serve it (instead of --to)")
	confirm := fs.Bool("confirm", false, "apply the move after the collision check")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsParkUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	domain, ok := requireOneArg(fs, stderr, prog, "domains move", "domain")
	if !ok {
		return exitUsage
	}
	req := apiclient.DomainLifecycleRequest{Domain: domain, From: *from, To: *to, FromEnvironment: *fromEnv, ToEnvironment: *toEnv, Confirm: *confirm}
	if (*from != "" && *fromSite != "") || (*to != "" && *toSite != "") {
		_, _ = fmt.Fprintf(stderr, "%s: use either --from or --from-static-site, and either --to or --to-static-site\n", prog)
		return exitUsage
	}
	if *fromSite != "" {
		req.From, req.FromKind = *fromSite, "static_site"
	}
	if *toSite != "" {
		req.To, req.ToKind = *toSite, "static_site"
	}
	if req.From == "" || req.To == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a source (--from or --from-static-site) and a target (--to or --to-static-site) are required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	res, err := client.MoveDomain(context.Background(), req)
	if err != nil {
		hint := ""
		if strings.Contains(err.Error(), "confirm") {
			hint = "; rerun with --confirm to move it"
		}
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("move domain %q: %w%s", domain, err, hint))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "moved %s from %s to %s\n", res.Domain, res.From, res.To)
	})
}

// runDomainsParked dispatches "domains parked <verb>".
func runDomainsParked(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsParkUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsParkUsage(prog))
		return exitOK
	case "list":
		return runDomainsParkedList(prog, args[1:], stdout, stderr, lookupEnv)
	case "attach":
		return runDomainsParkedAttach(prog, args[1:], stdout, stderr, lookupEnv)
	case "retention":
		return runDomainsParkedRetention(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runDomainsParkedDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains parked subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsParkUsage(prog))
		return exitUsage
	}
}

func runDomainsParkedList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains parked list", "print parked domains as a JSON array", stderr)
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	list, err := client.ListParkedDomains(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list parked domains: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no parked domains")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DOMAIN\tPARKED FROM\tKIND\tENV\tPARKED AT\tDNS\tTLS\tNOTE")
		for _, p := range list {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.Domain, p.ParkedFrom, p.ParkedFromKind, p.ParkedFromEnvironment, p.ParkedAt, parkedDNSCell(p), parkedTLSCell(p), p.Note)
		}
		_ = tw.Flush()
	})
}

func runDomainsParkedAttach(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains parked attach", "print the result as JSON", stderr)
	app := fs.String("app", "", "app that should serve the domain")
	site := fs.String("static-site", "", "static site that should serve the domain (instead of --app)")
	env := fs.String("env", "", "environment whose domain set receives the domain (default: the app's default list)")
	discard := fs.Bool("discard-settings", false, "drop the settings kept by park instead of restoring them")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsParkUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	domain, ok := requireOneArg(fs, stderr, prog, "domains parked attach", "domain")
	if !ok {
		return exitUsage
	}
	if (*app == "") == (*site == "") {
		_, _ = fmt.Fprintf(stderr, "%s: exactly one of --app or --static-site is required\n", prog)
		return exitUsage
	}
	req := apiclient.DomainLifecycleRequest{App: *app, Environment: *env, DiscardSettings: *discard}
	if *site != "" {
		req.App, req.TargetKind = *site, "static_site"
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	res, err := client.AttachParkedDomainTo(context.Background(), domain, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("attach parked domain %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "attached %s to %s\n", res.Domain, res.To)
		if len(res.RestoredSettings) > 0 {
			_, _ = fmt.Fprintf(stdout, "restored settings: %s\n", strings.Join(res.RestoredSettings, ", "))
		}
		if res.Message != "" {
			_, _ = fmt.Fprintln(stdout, res.Message)
		}
	})
}

func runDomainsParkedDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains parked delete", "print nothing; errors as JSON", stderr)
	token, apiURL, profile, jsonOut, _, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	domain, ok := requireOneArg(fs, stderr, prog, "domains parked delete", "domain")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	if err := client.DeleteParkedDomain(context.Background(), domain); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete parked domain %q: %w", domain, err))
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stdout, "deleted parked domain %s; its DNS records and certificate were not touched\n", domain)
	}
	return exitOK
}

func parkedDNSCell(p apiclient.ParkedDomain) string {
	if p.DNS == nil || p.DNS.Status == "" {
		return "unknown"
	}
	return p.DNS.Status
}

func parkedTLSCell(p apiclient.ParkedDomain) string {
	if p.TLS == nil || p.TLS.State == "" {
		return "none"
	}
	if p.TLS.DaysLeft == nil {
		return p.TLS.State
	}
	if *p.TLS.DaysLeft < 0 {
		return p.TLS.State + " (expired)"
	}
	return fmt.Sprintf("%s (%dd)", p.TLS.State, *p.TLS.DaysLeft)
}

func runDomainsParkedRetention(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains parked retention", "print the dry run as JSON", stderr)
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	plan, err := client.ParkedRetention(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("parked retention dry run: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, plan, func() {
		if plan.RetentionDays == 0 {
			_, _ = fmt.Fprintln(stdout, "parked domain cleanup is off (domains.parked_retention_days = 0)")
			return
		}
		_, _ = fmt.Fprintf(stdout, "parked domains older than %d days are deleted; next run %s\n", plan.RetentionDays, plan.NextRun)
		if len(plan.WouldDelete) == 0 {
			_, _ = fmt.Fprintln(stdout, "nothing to delete now")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DOMAIN\tPARKED AT\tAGE DAYS")
		for _, p := range plan.WouldDelete {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\n", p.Domain, p.ParkedAt, p.AgeDays)
		}
		_ = tw.Flush()
		if plan.TotalEligible > len(plan.WouldDelete) {
			_, _ = fmt.Fprintf(stdout, "%d more are due and go in later runs\n", plan.TotalEligible-len(plan.WouldDelete))
		}
	})
}
