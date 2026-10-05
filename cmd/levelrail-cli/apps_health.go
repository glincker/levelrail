package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/kit/probe"
)

// runAppsHealth dispatches "apps health <verb>", the CLI side of
// GET/PUT/DELETE /api/v1/apps/{name}/health.
func runAppsHealth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsHealthUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsHealthUsage(prog))
		return exitOK
	case "get":
		return runAppsHealthGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsHealthSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsHealthClear(prog, args[1:], stdout, stderr, lookupEnv)
	case "discover":
		return runAppsHealthDiscover(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps health subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsHealthUsage(prog))
		return exitUsage
	}
}

func appsHealthUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps health get <name> [flags]                                    show an app's readiness and liveness probes
  %[1]s apps health set <name> --probe readiness|liveness [probe flags]   set one probe, keeping the other as-is
  %[1]s apps health clear <name> [--probe readiness|liveness]             remove one probe, or both
  %[1]s apps health discover <name>                                      actively probe well-known paths, report what each one really did

A probe is either an HTTP(S) check (--path, with --scheme, --host,
--tls-skip-verify, --follow-redirects, --expected-status) or a command run
inside the container (--exec, exit 0 means healthy). --preset <name> fills
path/interval/timeout/failures from a common shortcut (healthz, health,
api-health, ping, status); any explicit flag passed alongside it wins. The
same settings can be declared in app.yaml's health: block.

Run "%[1]s apps health <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsHealthGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps health get", "print the health config as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps health get <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps health get", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	result, err := client.GetAppHealth(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get health for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printHealthHuman(stdout, result.Health) })
}

// probeFlags holds "apps health set"'s probe flags as typed.
type probeFlags struct {
	which, path, exec, scheme, host, followRedirects, expectedStatus, preset string
	interval, timeout, readyTimeout                                          string
	tlsSkipVerify                                                            bool
	failures                                                                 int
}

// healthPresetPaths are the same preset ids as the frontend's preset picker
// (web/src/lib/healthCheckDefaults.ts), so both surfaces offer identical
// one-click shortcuts.
var healthPresetPaths = map[string]string{
	"healthz":    "/healthz",
	"health":     "/health",
	"api-health": "/api/health",
	"ping":       "/ping",
	"status":     "/status",
}

const healthPresetNames = "healthz, health, api-health, ping, status"

// healthPresetTiming mirrors healthCheckDefaults.ts's probeTimingDefaults:
// readiness gates one deploy's cutover (short interval/timeout, no failure
// count), liveness restarts a hung container (longer interval, a few
// failures first). timeoutSeconds 0 means "leave it unset".
func healthPresetTiming(which string) (intervalSeconds, timeoutSeconds, failures int) {
	if which == "liveness" {
		return 30, 0, 3
	}
	return 5, 2, 0
}

func (f *probeFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.which, "probe", "", "which probe to set: readiness or liveness (required)")
	fs.StringVar(&f.path, "path", "", "HTTP path to request, e.g. /healthz")
	fs.StringVar(&f.exec, "exec", "", "command run inside the container through /bin/sh -c; exit 0 means healthy")
	fs.StringVar(&f.scheme, "scheme", "", "http (default) or https")
	fs.StringVar(&f.host, "host", "", "Host header (and TLS server name) to send")
	fs.BoolVar(&f.tlsSkipVerify, "tls-skip-verify", false, "accept a self-signed certificate (https only)")
	fs.StringVar(&f.followRedirects, "follow-redirects", "", "true or false; unset follows redirects")
	fs.StringVar(&f.expectedStatus, "expected-status", "", "accepted status codes, e.g. 200-399 or 200,204 (default 200-299)")
	fs.StringVar(&f.interval, "interval", "", "time between attempts, e.g. 5s")
	fs.StringVar(&f.timeout, "timeout", "", "per-attempt timeout, e.g. 2s")
	fs.IntVar(&f.failures, "failures", 0, "consecutive liveness failures before a restart")
	fs.StringVar(&f.readyTimeout, "ready-timeout", "", "how long a deploy waits for readiness, e.g. 90s")
	fs.StringVar(&f.preset, "preset", "", "fill path/interval/timeout/failures from a named preset ("+healthPresetNames+"); explicit flags always win")
}

func (f probeFlags) toProbe() (serviceProbe, error) {
	path, intervalStr, timeoutStr, failures := f.path, f.interval, f.timeout, f.failures
	if f.preset != "" {
		presetPath, ok := healthPresetPaths[f.preset]
		if !ok {
			return serviceProbe{}, fmt.Errorf("--preset: unknown preset %q (want one of %s)", f.preset, healthPresetNames)
		}
		if path == "" && f.exec == "" {
			path = presetPath
		}
		intervalSeconds, timeoutSeconds, presetFailures := healthPresetTiming(f.which)
		if intervalStr == "" {
			intervalStr = fmt.Sprintf("%ds", intervalSeconds)
		}
		if timeoutStr == "" && timeoutSeconds > 0 {
			timeoutStr = fmt.Sprintf("%ds", timeoutSeconds)
		}
		if failures == 0 && presetFailures > 0 {
			failures = presetFailures
		}
	}
	interval, err := parseDurationOrZero(intervalStr)
	if err != nil {
		return serviceProbe{}, fmt.Errorf("--interval: %w", err)
	}
	timeout, err := parseDurationOrZero(timeoutStr)
	if err != nil {
		return serviceProbe{}, fmt.Errorf("--timeout: %w", err)
	}
	p := serviceProbe{
		Path: path, Scheme: f.scheme, Host: f.host, TLSSkipVerify: f.tlsSkipVerify,
		ExpectedStatus: f.expectedStatus, Interval: interval.Nanoseconds(), Timeout: timeout.Nanoseconds(), Failures: failures,
	}
	if f.exec != "" {
		p.Exec = probe.ShellCommand(f.exec)
	}
	if f.followRedirects != "" {
		follow, err := strconv.ParseBool(f.followRedirects)
		if err != nil {
			return serviceProbe{}, fmt.Errorf("--follow-redirects must be true or false")
		}
		p.FollowRedirects = &follow
	}
	cfg := probe.Config{
		Path: p.Path, Scheme: p.Scheme, Host: p.Host, TLSSkipVerify: p.TLSSkipVerify, FollowRedirects: p.FollowRedirects,
		ExpectedStatus: p.ExpectedStatus, Exec: p.Exec, Interval: interval, Timeout: timeout,
	}
	if err := cfg.Validate(); err != nil {
		return serviceProbe{}, err
	}
	return p, nil
}

func runAppsHealthSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps health set", "print the resulting health config as JSON to stdout and nothing else", stderr)
	var pf probeFlags
	pf.register(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps health set <name> --probe readiness|liveness (--path PATH | --exec CMD) [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps health set", "app name")
	if !ok {
		return exitUsage
	}
	if pf.which != "readiness" && pf.which != "liveness" {
		_, _ = fmt.Fprintf(stderr, "%s: apps health set requires --probe readiness or --probe liveness\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	p, err := pf.toProbe()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s probe: %v", pf.which, err))
	}
	readyTimeout, err := parseDurationOrZero(pf.readyTimeout)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("--ready-timeout: %v", err))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	current, err := client.GetAppHealth(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get health for app %q: %w", name, err))
	}
	health := serviceHealth{}
	if current.Health != nil {
		health = *current.Health
	}
	if pf.which == "readiness" {
		health.Readiness = &p
	} else {
		health.Liveness = &p
	}
	if readyTimeout > 0 {
		health.ReadyTimeout = readyTimeout.Nanoseconds()
	}
	result, err := client.SetAppHealth(context.Background(), name, health)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set health for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printHealthHuman(stdout, result.Health) })
}

func runAppsHealthClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps health clear", "print the resulting health config as JSON to stdout and nothing else", stderr)
	var which string
	fs.StringVar(&which, "probe", "", "readiness or liveness; omit to clear both")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps health clear <name> [--probe readiness|liveness] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps health clear", "app name")
	if !ok {
		return exitUsage
	}
	if which != "" && which != "readiness" && which != "liveness" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--probe must be readiness or liveness"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if which == "" {
		if err := client.ClearAppHealth(context.Background(), name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear health for app %q: %w", name, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, appHealthResource{Name: name}, func() { printHealthHuman(stdout, nil) })
	}
	current, err := client.GetAppHealth(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get health for app %q: %w", name, err))
	}
	health := serviceHealth{}
	if current.Health != nil {
		health = *current.Health
	}
	if which == "readiness" {
		health.Readiness = nil
	} else {
		health.Liveness = nil
	}
	result, err := client.SetAppHealth(context.Background(), name, health)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear %s for app %q: %w", which, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printHealthHuman(stdout, result.Health) })
}

// runAppsHealthDiscover implements "apps health discover <name>", the
// CLI side of POST /api/v1/apps/{name}/health/discover: actively probes
// a fixed set of well-known paths against the app's own running
// container and prints each one's real outcome. Never guesses: nothing
// here is reported as working without having actually been tried.
func runAppsHealthDiscover(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps health discover", "print the probe attempts as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps health discover <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps health discover", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	result, err := client.DiscoverAppHealth(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("discover health for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printHealthDiscoveryTable(stdout, result) })
}

// printHealthDiscoveryTable renders every attempted path and its real
// outcome, not just whichever one (if any) looked like a match.
func printHealthDiscoveryTable(out io.Writer, result healthDiscoveryResponse) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PATH\tRESULT\tLATENCY\tDETAIL")
	for _, a := range result.Attempts {
		status := "failed"
		if a.Success {
			status = "ok"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%dms\t%s\n", a.Path, status, a.LatencyMs, dashIfEmpty(a.Error))
	}
	_ = tw.Flush()
	if result.Found != "" {
		_, _ = fmt.Fprintf(out, "\nfound a working health check at %s\n", result.Found)
		return
	}
	_, _ = fmt.Fprintln(out, "\nno single clear match; review the attempts above")
}

func printHealthHuman(out io.Writer, h *serviceHealth) {
	if h == nil || (h.Readiness == nil && h.Liveness == nil) {
		_, _ = fmt.Fprintln(out, "health:    no probes configured")
		return
	}
	printProbeHuman(out, "readiness", h.Readiness)
	printProbeHuman(out, "liveness", h.Liveness)
	if h.ReadyTimeout > 0 {
		_, _ = fmt.Fprintf(out, "ready timeout: %s\n", time.Duration(h.ReadyTimeout))
	}
}

func printProbeHuman(out io.Writer, label string, p *serviceProbe) {
	if p == nil {
		_, _ = fmt.Fprintf(out, "%s: not configured\n", label)
		return
	}
	_, _ = fmt.Fprintf(out, "%s: %s\n", label, describeProbe(*p))
}

// describeProbe renders p on one line, e.g. "GET https://:port/health (expect 200-399, follow redirects)".
func describeProbe(p serviceProbe) string {
	var b strings.Builder
	if len(p.Exec) > 0 {
		b.WriteString("exec " + probe.DescribeCommand(p.Exec))
	} else {
		scheme := p.Scheme
		if scheme == "" {
			scheme = probe.SchemeHTTP
		}
		host := ":port"
		if p.Host != "" {
			host = p.Host
		}
		fmt.Fprintf(&b, "GET %s://%s%s", scheme, host, p.Path)
		opts := []string{"expect " + defaultString(p.ExpectedStatus, probe.DefaultExpectedStatus)}
		if p.FollowRedirects != nil && !*p.FollowRedirects {
			opts = append(opts, "redirects not followed")
		}
		if p.TLSSkipVerify {
			opts = append(opts, "TLS verify off")
		}
		fmt.Fprintf(&b, " (%s)", strings.Join(opts, ", "))
	}
	if p.Interval > 0 {
		fmt.Fprintf(&b, " every %s", time.Duration(p.Interval))
	}
	if p.Timeout > 0 {
		fmt.Fprintf(&b, ", timeout %s", time.Duration(p.Timeout))
	}
	if p.Failures > 0 {
		fmt.Fprintf(&b, ", %d failures", p.Failures)
	}
	return b.String()
}

func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
