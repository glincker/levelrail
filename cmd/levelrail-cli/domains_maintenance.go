package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// domainsMaintenanceJSONUsage is every domains maintenance subcommand's
// --json flag description: identical across get/set/clear since each
// returns the same maintenance-state shape.
const domainsMaintenanceJSONUsage = "print the maintenance state as JSON to stdout and nothing else"

// runDomainsMaintenance dispatches "domains maintenance <verb> [flags]"
// to one of get/set/clear, mirroring runDomainsBasicAuth's own
// top-level dispatch shape for a different per-domain toggle.
func runDomainsMaintenance(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsMaintenanceUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsMaintenanceUsage(prog))
		return exitOK
	case "get":
		return runDomainsMaintenanceGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsMaintenanceSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsMaintenanceClear(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runDomainsMaintenanceStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "preview":
		return runDomainsMaintenancePreview(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains maintenance subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsMaintenanceUsage(prog))
		return exitUsage
	}
}

func domainsMaintenanceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains maintenance get <app> <domain> [flags]     show a domain's maintenance state
  %[1]s domains maintenance set <app> <domain> [flags]     enable maintenance mode (page text, end time, bypass)
  %[1]s domains maintenance clear <app> <domain> [flags]   disable maintenance mode
  %[1]s domains maintenance status [flags]                 list every domain in maintenance, with how long
  %[1]s domains maintenance preview [flags]                show the page visitors get, without enabling anything

While enabled, the embedded Caddy ingress serves a fixed "down for
maintenance" response for <domain> instead of proxying to its
container, without stopping the container itself. Visitors get a 503 page with a Retry-After header;
scripts, styles and images are not served. Pass --for, --until or
--max-duration so it cannot be left on by accident. <domain> must
already be one of <app>'s configured domains (see "%[1]s apps get <app>"
or "%[1]s domains list").

Run "%[1]s domains maintenance <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsMaintenanceGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance get", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance get <app> <domain> [flags]\n\nShows a domain's currently configured maintenance state.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains maintenance get", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	m, err := client.GetDomainMaintenance(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get maintenance state for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, m, func() { printDomainMaintenanceHuman(stdout, m) })
}

func runDomainsMaintenanceSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance set", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance set <app> <domain> [flags]\n\nEnables maintenance mode on <domain>. Running it again replaces the settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	opts := maintenanceSetFlags(fs)

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains maintenance set", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	req, err := opts.request(time.Now())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: domains maintenance set: %v\n", prog, err)
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	m, err := client.SetDomainMaintenanceOptions(context.Background(), appName, domain, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("enable maintenance mode for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, m, func() { printDomainMaintenanceHuman(stdout, m) })
}

func runDomainsMaintenanceClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance clear", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance clear <app> <domain> [flags]\n\nDisables maintenance mode on <domain>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains maintenance clear", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	m, err := client.ClearDomainMaintenance(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable maintenance mode for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, m, func() {
		_, _ = fmt.Fprintf(stdout, "maintenance mode disabled for domain %q\n", domain)
	})
}

func printDomainMaintenanceHuman(out io.Writer, m domainMaintenanceResource) {
	status := "disabled"
	if m.Enabled {
		status = "enabled"
	}
	_, _ = fmt.Fprintf(out, "domain: %s\nstatus: %s\n", m.Domain, status)
	if !m.Enabled {
		return
	}
	if m.Title != "" {
		_, _ = fmt.Fprintf(out, "title: %s\n", m.Title)
	}
	if m.Message != "" {
		_, _ = fmt.Fprintf(out, "message: %s\n", m.Message)
	}
	if m.RetryAfterSeconds > 0 {
		_, _ = fmt.Fprintf(out, "retry after: %ds\n", m.RetryAfterSeconds)
	}
	if m.StartedAt != "" {
		_, _ = fmt.Fprintf(out, "started: %s\n", m.StartedAt)
	}
	if m.Until != "" {
		_, _ = fmt.Fprintf(out, "ends: %s (scheduled window %s)\n", m.Until, m.WindowID)
	}
	if m.AutoOffAt != "" {
		_, _ = fmt.Fprintf(out, "switches off on its own at: %s\n", m.AutoOffAt)
	}
	if m.Until == "" && m.AutoOffAt == "" {
		_, _ = fmt.Fprintln(out, "ends: never on its own, run \"domains maintenance clear\" or pass --max-duration")
	}
	if len(m.Bypass) > 0 {
		_, _ = fmt.Fprintf(out, "bypass: %s\n", strings.Join(m.Bypass, ", "))
	}
	_, _ = fmt.Fprintf(out, "health check bypass: %t\n", m.BypassHealthCheck)
}

// maintenanceSetOptions are the page, end time and bypass flags of set.
type maintenanceSetOptions struct {
	title, message, until, bypass, maxDur *string
	retryAfter, forDur                    *time.Duration
	noHealthBypass                        *bool
}

func maintenanceSetFlags(fs interface {
	String(name, value, usage string) *string
	Duration(name string, value time.Duration, usage string) *time.Duration
	Bool(name string, value bool, usage string) *bool
}) maintenanceSetOptions {
	return maintenanceSetOptions{
		title:          fs.String("title", "", "maintenance page title (plain text, up to 120 characters)"),
		message:        fs.String("message", "", "maintenance page message (plain text, up to 2000 characters)"),
		until:          fs.String("until", "", "end maintenance automatically at this RFC 3339 time"),
		forDur:         fs.Duration("for", 0, "end maintenance automatically after this long, e.g. 2h (alternative to --until)"),
		retryAfter:     fs.Duration("retry-after", 0, "Retry-After sent with the 503, e.g. 10m (default 5m)"),
		maxDur:         fs.String("max-duration", "", "switch maintenance off after this long when no end time is set, e.g. 4h; 0 means no limit (default: the server's APP_MAINTENANCE_MAX_DURATION)"),
		bypass:         fs.String("bypass", "", "comma separated IP addresses or CIDRs that still reach the app"),
		noHealthBypass: fs.Bool("no-health-bypass", false, "also show the page on the app's readiness path (by default uptime probes still reach it)"),
	}
}

func (o maintenanceSetOptions) request(now time.Time) (apiclient.SetDomainMaintenanceRequest, error) {
	req := apiclient.SetDomainMaintenanceRequest{Title: *o.title, Message: *o.message, RetryAfterSeconds: int(o.retryAfter.Seconds())}
	if *o.retryAfter < 0 || *o.forDur < 0 {
		return req, errors.New("--retry-after and --for must not be negative")
	}
	switch {
	case *o.until != "" && *o.forDur > 0:
		return req, errors.New("use either --until or --for, not both")
	case *o.until != "":
		t, err := time.Parse(time.RFC3339, *o.until)
		if err != nil {
			return req, fmt.Errorf("--until %q is not an RFC 3339 time", *o.until)
		}
		req.Until = t.UTC().Format(time.RFC3339)
	case *o.forDur > 0:
		req.Until = now.Add(*o.forDur).UTC().Format(time.RFC3339)
	}
	for _, b := range strings.Split(*o.bypass, ",") {
		if b = strings.TrimSpace(b); b != "" {
			req.Bypass = append(req.Bypass, b)
		}
	}
	if *o.maxDur != "" {
		d, err := time.ParseDuration(*o.maxDur)
		if err != nil || d < 0 {
			return req, fmt.Errorf("--max-duration %q is not a duration like 4h", *o.maxDur)
		}
		secs := int(d.Seconds())
		req.MaxDurationSeconds = &secs
	}
	if *o.noHealthBypass {
		off := false
		req.BypassHealthCheck = &off
	}
	return req, nil
}

func runDomainsMaintenanceStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance status", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance status [flags]\n\nLists every domain currently in maintenance across all apps.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ListFleetMaintenance(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list domains in maintenance: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printFleetMaintenance(stdout, res) })
}

func printFleetMaintenance(out io.Writer, res apiclient.FleetMaintenanceResource) {
	if len(res.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no domain is in maintenance")
		return
	}
	for _, e := range res.Items {
		ends := "never on its own"
		switch {
		case e.Until != "":
			ends = "ends " + e.Until
		case e.AutoOffAt != "":
			ends = "auto off " + e.AutoOffAt
		}
		warn := ""
		if res.AttentionAfterSeconds > 0 && e.ElapsedSeconds > res.AttentionAfterSeconds {
			warn = "  (longer than expected)"
		}
		_, _ = fmt.Fprintf(out, "%s  app=%s  on for %s  %s%s\n", e.Domain, e.App, (time.Duration(e.ElapsedSeconds) * time.Second).Round(time.Minute), ends, warn)
	}
}

func runDomainsMaintenancePreview(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains maintenance preview", domainsMaintenanceJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains maintenance preview [flags]\n\nPrints the status, Retry-After and HTML visitors would get. Changes nothing.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	title := fs.String("title", "", "maintenance page title")
	message := fs.String("message", "", "maintenance page message")
	retry := fs.Duration("retry-after", 0, "Retry-After, e.g. 10m (default 5m)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.PreviewMaintenancePage(context.Background(), *title, *message, int(retry.Seconds()))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview maintenance page: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "status: %d\nretry-after: %ss\n\n%s", res.Status, res.RetryAfter, res.HTML)
	})
}
