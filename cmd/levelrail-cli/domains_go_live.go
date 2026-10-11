package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	goLiveStateLive   = "live"
	goLiveStateFailed = "failed"
	defaultGoLiveWait = 2 * time.Minute
	defaultGoLivePoll = 3 * time.Second
)

// goLiveSleep and goLivePollInterval are variables so tests do not wait.
var (
	goLiveSleep        = time.Sleep
	goLivePollInterval = defaultGoLivePoll
)

type goLiveResult = apiclient.GoLiveResult

func domainsGoLiveUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains go-live <app> <domain> [--dns auto|off|preview] [--replace] [--plan] [--wait] [--timeout 2m] [flags]
  %[1]s domains go-live runs [--limit N] [flags]     recent automation runs
  %[1]s domains go-live undo <run-id> [flags]        undo what one run created
  %[1]s domains backfill-base-domain [--confirm] [flags]

go-live adds the domain to the app when needed, creates its DNS record at the
connected provider, and reports each step (dns, propagation, certificate,
http). --wait keeps polling until the domain is live and exits non-zero when
it is not within --timeout. --plan only shows what would happen.
`, prog)
}

// runDomainsGoLive dispatches "domains go-live ...".
func runDomainsGoLive(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsGoLiveUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsGoLiveUsage(prog))
		return exitOK
	case "runs":
		return runGoLiveRuns(prog, args[1:], stdout, stderr, lookupEnv)
	case "undo":
		return runGoLiveUndo(prog, args[1:], stdout, stderr, lookupEnv)
	}
	return runGoLiveRun(prog, args, stdout, stderr, lookupEnv)
}

func runGoLiveRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains go-live", "print the result as JSON to stdout and nothing else", stderr)
	var dns string
	var replace, plan, wait bool
	var timeout time.Duration
	fs.StringVar(&dns, "dns", "", "automatic DNS: auto (default), off or preview")
	fs.BoolVar(&replace, "replace", false, "overwrite a conflicting A/AAAA/CNAME record")
	fs.BoolVar(&plan, "plan", false, "show the planned steps without changing anything")
	fs.BoolVar(&wait, "wait", false, "keep polling until the domain is live")
	fs.DurationVar(&timeout, "timeout", defaultGoLiveWait, "how long --wait polls")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsGoLiveUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains go-live", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	app, domain := rest[0], strings.ToLower(rest[1])
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if plan {
		out, err := client.PlanGoLive(ctx, app, apiclient.GoLivePlanRequest{Domain: domain})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan go-live for %q: %w", domain, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, out, func() {
			for _, p := range out.Plans {
				printGoLive(stdout, p)
			}
		})
	}
	res, err := client.RunGoLive(ctx, app, domain, apiclient.GoLiveRequest{DNS: dns, Replace: replace})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("go-live for %q: %w", domain, err))
	}
	if wait {
		res, err = waitGoLive(ctx, client, res, timeout, stdout, jsonOut)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}
	code := writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if !wait {
			printGoLive(stdout, res)
		}
	})
	if code == exitOK && wait && res.State != goLiveStateLive {
		return exitAPIError
	}
	return code
}

// printGoLive writes a result's steps as text.
func printGoLive(out io.Writer, res goLiveResult) {
	_, _ = fmt.Fprintf(out, "%s: %s\n", res.Domain, res.State)
	for _, s := range res.Steps {
		printGoLiveStep(out, s)
	}
	if res.State == goLiveStateLive && res.URL != "" {
		_, _ = fmt.Fprintf(out, "  live at %s\n", res.URL)
	}
}

func printGoLiveStep(out io.Writer, s apiclient.GoLiveStep) {
	line := fmt.Sprintf("  %-12s %-9s", s.ID, s.State)
	if s.Detail != "" {
		line += " " + s.Detail
	}
	_, _ = fmt.Fprintln(out, line)
	for _, r := range s.Resolvers {
		answer := strings.Join(r.Addresses, ", ")
		if answer == "" {
			answer = r.Error
		}
		_, _ = fmt.Fprintf(out, "      %s: %s\n", r.Name, answer)
	}
}

// waitGoLive polls GET .../go-live until the domain is live, a step failed,
// or timeout passes, printing each step the first time its state changes.
func waitGoLive(ctx context.Context, client *Client, res goLiveResult, timeout time.Duration, stdout io.Writer, quiet bool) (goLiveResult, error) {
	deadline := time.Now().Add(timeout)
	seen := map[string]string{}
	for {
		if !quiet {
			for _, s := range res.Steps {
				key := s.State + "|" + s.Detail
				if seen[s.ID] != key {
					seen[s.ID] = key
					printGoLiveStep(stdout, s)
				}
			}
		}
		switch res.State {
		case goLiveStateLive:
			if !quiet && res.URL != "" {
				_, _ = fmt.Fprintf(stdout, "live at %s\n", res.URL)
			}
			return res, nil
		case goLiveStateFailed:
			return res, nil
		}
		if !time.Now().Before(deadline) {
			return res, nil
		}
		goLiveSleep(goLivePollInterval)
		next, err := client.GetGoLive(ctx, res.App, res.Domain)
		if err != nil {
			return res, fmt.Errorf("poll go-live for %q: %w", res.Domain, err)
		}
		next.RunID = res.RunID
		res = next
	}
}

func runGoLiveRuns(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains go-live runs", "print the runs as JSON to stdout and nothing else", stderr)
	var limit int
	fs.IntVar(&limit, "limit", 20, "how many runs to show")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	runs, err := client.ListAutomationRuns(context.Background(), limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list automation runs: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, runs, func() {
		if len(runs) == 0 {
			_, _ = fmt.Fprintln(stdout, "no automation runs yet")
		}
		for _, r := range runs {
			undone := ""
			if r.UndoneAt != nil {
				undone = " (undone)"
			}
			_, _ = fmt.Fprintf(stdout, "%s  %s  %s/%s  %s%s\n", r.ID, r.CreatedAt.Format(time.RFC3339), r.App, r.Domain, r.Result, undone)
		}
	})
}

func runGoLiveUndo(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains go-live undo", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsGoLiveUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains go-live undo", "a run id", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.UndoAutomationRun(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("undo run %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		for _, r := range res.Reverted {
			_, _ = fmt.Fprintln(stdout, "reverted: "+r)
		}
		for _, r := range res.Skipped {
			_, _ = fmt.Fprintln(stdout, "could not revert: "+r)
		}
	})
}

// runDomainsBackfillBaseDomain implements "domains backfill-base-domain".
func runDomainsBackfillBaseDomain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains backfill-base-domain", "print the result as JSON to stdout and nothing else", stderr)
	var confirm bool
	fs.BoolVar(&confirm, "confirm", false, "apply the change (the default is a dry run)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsGoLiveUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.BackfillBaseDomain(context.Background(), confirm)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("backfill apps base domain: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.DryRun {
			_, _ = fmt.Fprintf(stdout, "dry run for base domain %s (pass --confirm to apply)\n", res.BaseDomain)
		}
		for _, it := range res.Items {
			_, _ = fmt.Fprintf(stdout, "%-24s %-40s %s\n", it.App, it.Domain, it.Status)
		}
		if len(res.Items) == 0 {
			_, _ = fmt.Fprintln(stdout, "every app already has a domain")
		}
	})
}

func runSettingsDomainAutomation(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsDomainAutomationUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsDomainAutomationUsage(prog))
		return exitOK
	case "get":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.DomainAutomationResource]{
			cmdLabel:  "settings domain-automation get",
			jsonUsage: "print the policy as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings domain-automation get [flags]\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (apiclient.DomainAutomationResource, error) {
				return c.GetDomainAutomation(ctx)
			},
			errVerb: "get domain automation",
			print:   printDomainAutomation,
		})
	case "set":
		return runSettingsDomainAutomationSet(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown settings domain-automation subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, settingsDomainAutomationUsage(prog))
	return exitUsage
}

func settingsDomainAutomationUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings domain-automation get [flags]
  %[1]s settings domain-automation set [--auto-dns=BOOL] [--auto-proxy-route=BOOL] [--force-https=BOOL]
      [--www-policy off|redirect_to_apex|redirect_to_www] [--attach-www-counterpart=BOOL]
      [--wildcard-for-base-domain=BOOL] [--verify-after=BOOL] [flags]

The policy applied when a domain is added to an app. Writes need the root ability.
`, prog)
}

func printDomainAutomation(out io.Writer, r apiclient.DomainAutomationResource) {
	_, _ = fmt.Fprintf(out, "auto_dns:                 %v (provider: %s)\n", r.AutoDNS, r.DNSProvider)
	_, _ = fmt.Fprintf(out, "auto_proxy_route:         %v (proxy integration: %v)\n", r.AutoProxyRoute, r.ProxyIntegration)
	_, _ = fmt.Fprintf(out, "force_https:              %v\n", r.ForceHTTPS)
	_, _ = fmt.Fprintf(out, "www_policy:               %s\n", r.WWWPolicy)
	_, _ = fmt.Fprintf(out, "attach_www_counterpart:   %v\n", r.AttachWWWCounterpart)
	_, _ = fmt.Fprintf(out, "wildcard_for_base_domain: %v\n", r.WildcardForBaseDomain)
	_, _ = fmt.Fprintf(out, "verify_after:             %v\n", r.VerifyAfter)
}

func runSettingsDomainAutomationSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings domain-automation set", "print the policy as JSON to stdout and nothing else", stderr)
	var autoDNS, autoProxy, forceHTTPS, attachWWW, wildcard, verify bool
	var wwwPolicy string
	fs.BoolVar(&autoDNS, "auto-dns", false, "create DNS records automatically")
	fs.BoolVar(&autoProxy, "auto-proxy-route", false, "wait for the managed proxy route")
	fs.BoolVar(&forceHTTPS, "force-https", false, "force HTTPS for the domain")
	fs.StringVar(&wwwPolicy, "www-policy", "", "off, redirect_to_apex or redirect_to_www")
	fs.BoolVar(&attachWWW, "attach-www-counterpart", false, "attach the www or apex partner host")
	fs.BoolVar(&wildcard, "wildcard-for-base-domain", false, "create a wildcard record for the apps base domain")
	fs.BoolVar(&verify, "verify-after", false, "probe propagation, certificate and http after adding")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsDomainAutomationUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := client.GetDomainAutomation(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get current domain automation: %w", err))
	}
	p := cur.DomainAutomationPolicy
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "auto-dns":
			p.AutoDNS = autoDNS
		case "auto-proxy-route":
			p.AutoProxyRoute = autoProxy
		case "force-https":
			p.ForceHTTPS = forceHTTPS
		case "www-policy":
			p.WWWPolicy = wwwPolicy
		case "attach-www-counterpart":
			p.AttachWWWCounterpart = attachWWW
		case "wildcard-for-base-domain":
			p.WildcardForBaseDomain = wildcard
		case "verify-after":
			p.VerifyAfter = verify
		}
	})
	res, err := client.UpdateDomainAutomation(ctx, policyOverride(p))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set domain automation: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDomainAutomation(stdout, res) })
}

func policyOverride(p apiclient.DomainAutomationPolicy) apiclient.DomainAutomationOverride {
	return apiclient.DomainAutomationOverride{
		AutoDNS: &p.AutoDNS, AutoProxyRoute: &p.AutoProxyRoute, ForceHTTPS: &p.ForceHTTPS,
		WWWPolicy: &p.WWWPolicy, AttachWWWCounterpart: &p.AttachWWWCounterpart,
		WildcardForBaseDomain: &p.WildcardForBaseDomain, VerifyAfter: &p.VerifyAfter,
	}
}
