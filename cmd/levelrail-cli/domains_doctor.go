package main

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
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
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains doctor", doctorStrings().render("domains_doctor_flag_json"), stderr)
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
	cat := doctorStrings()
	_, _ = fmt.Fprintf(out, "%s %s (%s)\n", cat.render("domains_doctor_title"), rep.Domain, rep.Status)
	_, _ = fmt.Fprintln(out, cat.render("domains_doctor_checked", "at", rep.CheckedAt.Local().Format(time.DateTime), "fixes", strconv.Itoa(len(fixes)), "passed", strconv.Itoa(passed)))
	if rep.ProbeNote != "" {
		_, _ = fmt.Fprintf(out, "%s %s\n", cat.render("domains_doctor_note"), rep.ProbeNote)
	}
	for i, c := range fixes {
		_, _ = fmt.Fprintf(out, "\n%d. [%s] %s\n", i+1, c.State, c.Title)
		printDoctorExplanation(out, c)
		if c.Fix == nil {
			continue
		}
		_, _ = fmt.Fprintf(out, "   %s %s\n", cat.render("domains_doctor_fix"), c.Fix.Summary)
		for j, step := range c.Fix.Steps {
			_, _ = fmt.Fprintf(out, "     %d. %s\n", j+1, step)
		}
		if a := c.Fix.Action; a != nil {
			switch {
			case a.Value != "":
				_, _ = fmt.Fprintf(out, "   %s: %s\n", a.Label, a.Value)
			case a.API != "":
				_, _ = fmt.Fprintf(out, "   %s: %s\n", a.Label, a.API)
			}
		}
		if c.Fix.CLIHint != "" {
			_, _ = fmt.Fprintf(out, "   %s %s\n", cat.render("domains_doctor_terminal"), c.Fix.CLIHint)
		}
	}
	if len(fixes) == 0 {
		_, _ = fmt.Fprintln(out, "\n"+cat.render("domains_doctor_nothing"))
	}
}

// printDoctorExplanation prints the plain cause and the observed facts, and
// falls back to the raw detail line for a report from an older server.
func printDoctorExplanation(out io.Writer, c apiclient.DoctorCheck) {
	if c.Cause == "" {
		if c.Detail != "" {
			_, _ = fmt.Fprintf(out, "   %s\n", c.Detail)
		}
		return
	}
	_, _ = fmt.Fprintf(out, "   %s\n", c.Cause)
	for _, e := range c.Evidence {
		_, _ = fmt.Fprintf(out, "   - %s\n", e)
	}
}

func domainsDoctorUsage(prog string) string {
	return doctorStrings().render("domains_doctor_usage", "prog", prog, "token_env", apiTokenEnvHelp, "api_env", apiURLEnvHelp, "api_url", defaultAPIURL)
}

// runDomainsSummary implements "domains summary".
func runDomainsSummary(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cat := doctorStrings()
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains summary", cat.render("domains_summary_flag_json"), stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, cat.render("domains_summary_usage", "prog", prog))
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
		_, _ = fmt.Fprintf(tw, "%s\t%d\n", cat.render("domains_summary_attention"), sum.Attention)
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", cat.render("domains_summary_domains"), cat.render("domains_summary_domains_line",
			"total", strconv.Itoa(d.Total), "live", strconv.Itoa(d.Live), "propagating", strconv.Itoa(d.Propagating),
			"waiting", strconv.Itoa(d.Waiting), "not_set_up", strconv.Itoa(d.NotSetUp), "paused", strconv.Itoa(d.Paused), "unknown", strconv.Itoa(d.Unknown)))
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", cat.render("domains_summary_certs"), cat.render("domains_summary_certs_line",
			"expiring", strconv.Itoa(c.Expiring), "days", strconv.Itoa(c.WindowDays), "expired", strconv.Itoa(c.Expired), "failing", strconv.Itoa(c.RenewalFailing)))
		_ = tw.Flush()
	})
}

// runDomainsActivity implements "domains activity <domain>".
func runDomainsActivity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cat := doctorStrings()
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains activity", cat.render("domains_activity_flag_json"), stderr)
	var limit int
	var before, actions string
	fs.IntVar(&limit, "limit", 0, "events per page (default: server default)")
	fs.StringVar(&before, "before", "", "cursor from a previous page's next_cursor")
	fs.StringVar(&actions, "actions", "", `comma separated action prefixes, e.g. "dns_record.,proxy_route."`)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, cat.render("domains_activity_usage", "prog", prog))
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
			_, _ = fmt.Fprintln(stdout, cat.render("domains_activity_none"))
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, cat.render("domains_activity_header"))
		for _, e := range page.Events {
			title := e.Title
			if e.Detail != "" {
				title += ": " + e.Detail
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.At.Local().Format(time.DateTime), e.Kind, title, e.Actor.Name)
		}
		_ = tw.Flush()
		if page.NextCursor != "" {
			_, _ = fmt.Fprintf(stdout, "\n%s\n", cat.render("domains_activity_more", "cursor", page.NextCursor))
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
	case "domain_traffic_drop":
		window, pct, minReq := r.ForDuration, int(r.Threshold), r.RestartCountThreshold
		if window == "" {
			window = "1h"
		}
		if pct <= 0 {
			pct = 50
		}
		if minReq <= 0 {
			minReq = 20
		}
		return fmt.Sprintf("requests in %s under %d%% of the same window yesterday (baseline of at least %d)", window, pct, minReq)
	case "parked_domain_cert_expiry":
		days := int(r.Threshold)
		if days <= 0 {
			days = 14
		}
		return fmt.Sprintf("a parked domain certificate with fewer than %d days left", days)
	}
	return "-"
}
