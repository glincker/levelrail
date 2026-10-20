package main

import (
	"context"
	"fmt"
	"io"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Doctor check states as the API reports them.
const (
	doctorStateFail = "fail"
	doctorStateWarn = "warn"
	doctorStatePass = "pass"
)

// runDomainsDoctor implements "domains doctor <app> <domain>".
func runDomainsDoctor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains doctor", "print the doctor report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsDoctorUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains doctor", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rep, err := client.DomainDoctor(context.Background(), rest[0], rest[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("doctor %q for app %q: %w", rest[1], rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printDoctorReport(stdout, rep) })
}

// prioritizedChecks orders failures before warnings, then by tier.
func prioritizedChecks(checks []apiclient.DoctorCheck) []apiclient.DoctorCheck {
	var out []apiclient.DoctorCheck
	for _, c := range checks {
		if c.State == doctorStateFail || c.State == doctorStateWarn {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b apiclient.DoctorCheck) int {
		if a.State != b.State {
			if a.State == doctorStateFail {
				return -1
			}
			return 1
		}
		return a.Tier - b.Tier
	})
	return out
}

func printDoctorReport(out io.Writer, rep apiclient.DomainDoctorReport) {
	passed := 0
	for _, c := range rep.Checks {
		if c.State == doctorStatePass {
			passed++
		}
	}
	fixes := prioritizedChecks(rep.Checks)
	_, _ = fmt.Fprintf(out, "Doctor: %s (%s)\n", rep.Domain, rep.Status)
	_, _ = fmt.Fprintf(out, "checked %s, %d to fix, %d passed\n", rep.CheckedAt.Local().Format(time.DateTime), len(fixes), passed)
	if rep.ProbeNote != "" {
		_, _ = fmt.Fprintf(out, "note: %s\n", rep.ProbeNote)
	}
	for i, c := range fixes {
		_, _ = fmt.Fprintf(out, "\n%d. [%s] %s\n", i+1, c.State, c.Title)
		if c.Detail != "" {
			_, _ = fmt.Fprintf(out, "   %s\n", c.Detail)
		}
		if c.Fix == nil {
			continue
		}
		_, _ = fmt.Fprintf(out, "   fix: %s\n", c.Fix.Summary)
		if a := c.Fix.Action; a != nil {
			switch {
			case a.Value != "":
				_, _ = fmt.Fprintf(out, "   %s: %s\n", a.Label, a.Value)
			case a.API != "":
				_, _ = fmt.Fprintf(out, "   %s: %s\n", a.Label, a.API)
			}
		}
	}
	if len(fixes) == 0 {
		_, _ = fmt.Fprintln(out, "\nnothing to fix")
	}
}

func domainsDoctorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains doctor <app> <domain> [flags]

Runs every domain check (DNS per resolver, CAA, ports 80 and 443, TLS
certificate, HTTP status, redirects, HSTS, proxy ownership, app state,
certificate renewal) and prints a prioritised fix list. It only connects to
<domain> when the domain resolves to this server; otherwise it runs the DNS
checks and says why.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the report as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runDomainsSummary implements "domains summary".
func runDomainsSummary(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains summary", "print the summary as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains summary [--json]\n\nCounts domains by status, certificates expiring or failing, and the total needing attention.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	sum, err := client.TrafficSummary(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("traffic summary: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, sum, func() {
		d, c := sum.Domains, sum.Certificates
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintf(tw, "needs attention\t%d\n", sum.Attention)
		_, _ = fmt.Fprintf(tw, "domains\t%d (live %d, propagating %d, waiting for DNS %d, not set up %d, paused %d, unknown %d)\n",
			d.Total, d.Live, d.Propagating, d.Waiting, d.NotSetUp, d.Paused, d.Unknown)
		_, _ = fmt.Fprintf(tw, "certificates\t%d expiring within %d days, %d expired, %d failing to renew\n", c.Expiring, c.WindowDays, c.Expired, c.RenewalFailing)
		_ = tw.Flush()
	})
}

// runDomainsActivity implements "domains activity <domain>".
func runDomainsActivity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains activity", "print the activity page as JSON to stdout and nothing else", stderr)
	var limit int
	var before, actions string
	fs.IntVar(&limit, "limit", 0, "events per page (default: server default)")
	fs.StringVar(&before, "before", "", "cursor from a previous page's next_cursor")
	fs.StringVar(&actions, "actions", "", `comma separated action prefixes, e.g. "dns_record.,proxy_route."`)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains activity <domain> [--limit N] [--before CURSOR] [--actions PREFIXES] [--json]\n\nLists a domain's recent changes and certificate events, newest first.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains activity", "a domain", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	page, err := client.DomainActivity(context.Background(), rest[0], limit, before, actions)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("activity for %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, page, func() {
		if len(page.Events) == 0 {
			_, _ = fmt.Fprintln(stdout, "no activity")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TIME\tKIND\tEVENT\tBY")
		for _, e := range page.Events {
			title := e.Title
			if e.Detail != "" {
				title += ": " + e.Detail
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.At.Local().Format(time.DateTime), e.Kind, title, e.Actor.Name)
		}
		_ = tw.Flush()
		if page.NextCursor != "" {
			_, _ = fmt.Fprintf(stdout, "\nmore: --before %s\n", page.NextCursor)
		}
	})
}

// trafficAlertCondition describes the domain and certificate alert kinds.
func trafficAlertCondition(r alertRuleResource) string {
	switch r.Kind {
	case "cert_expiring":
		days := int(r.Threshold)
		if days <= 0 {
			days = 14
		}
		return fmt.Sprintf("any certificate expiring within %d days", days)
	case "cert_renewal_stalled":
		return "a certificate renewal stalled or an ACME failure persisting"
	case "domain_not_resolving":
		window := r.ForDuration
		if window == "" {
			window = "30m"
		}
		return "a domain not resolving for " + window
	}
	return "-"
}
