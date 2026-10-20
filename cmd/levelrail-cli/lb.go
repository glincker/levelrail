package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// runLB dispatches "lb <verb>": the CLI counterpart of the
// /api/v1/apps/{name}/loadbalancer routes.
func runLB(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, lbUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, lbUsage(prog))
		return exitOK
	case "list":
		return runLBList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runLBShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runLBSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runLBClear(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runLBStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "check":
		return runLBCheck(prog, args[1:], stdout, stderr, lookupEnv)
	case "history":
		return runLBHistory(prog, args[1:], stdout, stderr, lookupEnv)
	case "upstream":
		return runLBUpstream(prog, args[1:], stdout, stderr, lookupEnv)
	case "export":
		return runLBExport(prog, args[1:], stdout, stderr, lookupEnv)
	case "import":
		return runLBImport(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown lb subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, lbUsage(prog))
		return exitUsage
	}
}

func lbUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s lb list [--state S] [--search Q] [flags]       every load balancer across apps, with upstream health
  %[1]s lb show <app> [flags]                          show an app's load balancer config
  %[1]s lb set <app> [--algorithm ...] [flags]         create or change the load balancer (only the flags you pass change)
  %[1]s lb clear <app> [flags]                         remove the load balancer, back to a single upstream
  %[1]s lb status <app> [flags]                        live upstream table: health, active requests, last check
  %[1]s lb check <app> [flags]                         probe every upstream once, right now
  %[1]s lb history <app> [--limit N] [flags]           recent checks and state changes per upstream
  %[1]s lb upstream <app> <id> --state S [flags]       set an upstream active, draining or disabled
  %[1]s lb export <app> --format FORMAT [--out FILE]   generate terraform, cdk, cloudformation, caddy or caddy-json
  %[1]s lb import <app> --file app.yaml [--service S]  load the loadbalancer: block of an app.yaml

Run "%[1]s lb <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func printLBHuman(out io.Writer, r apiclient.LoadBalancerResource) {
	_, _ = fmt.Fprintf(out, "app_name:   %s\n", r.AppName)
	if !r.Configured || r.Config == nil {
		_, _ = fmt.Fprintln(out, "loadbalancer: not configured (single upstream)")
		return
	}
	c := r.Config
	algo := c.Algorithm
	if algo == "" {
		algo = "round_robin"
	}
	_, _ = fmt.Fprintf(out, "algorithm:  %s\n", algo)
	if len(c.Weights) > 0 {
		_, _ = fmt.Fprintf(out, "weights:    %v\n", c.Weights)
	}
	if c.CookieName != "" {
		_, _ = fmt.Fprintf(out, "cookie:     %s\n", c.CookieName)
	}
	if h := c.ActiveHealth; h != nil {
		_, _ = fmt.Fprintf(out, "health:     GET %s every %s (timeout %s)\n", h.Path, orDash(h.Interval), orDash(h.Timeout))
	}
	if p := c.PassiveHealth; p != nil {
		_, _ = fmt.Fprintf(out, "passive:    %d fails for %s\n", p.MaxFails, orDash(p.FailDuration))
	}
	if r := c.Retries; r != nil {
		_, _ = fmt.Fprintf(out, "retries:    %d within %s\n", r.Count, orDash(r.TryDuration))
	}
	for _, kv := range [][2]string{{"slow_start", c.SlowStart}, {"drain", c.DrainTimeout}, {"timeout", c.RequestTimeout}} {
		if kv[1] != "" {
			_, _ = fmt.Fprintf(out, "%-11s %s\n", kv[0]+":", kv[1])
		}
	}
	if rl := c.RateLimit; rl != nil {
		_, _ = fmt.Fprintf(out, "rate_limit: %d rps, burst %d\n", rl.RPS, rl.Burst)
	}
	if c.UpstreamTLS != nil {
		_, _ = fmt.Fprintln(out, "upstream:   https")
	}
}

func runLBShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb show", "print the config as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb show <app> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "lb show", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.GetLoadBalancer(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get load balancer for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printLBHuman(stdout, res) })
}

func runLBClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb clear", "print {\"cleared\": true} as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb clear <app> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "lb clear", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if err := client.DeleteLoadBalancer(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear load balancer for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "load balancer cleared for app %q\n", name)
	})
}

func runLBStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb status", "print the status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb status <app> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "lb status", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	st, err := client.GetLoadBalancerStatus(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get load balancer status for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, st, func() {
		_, _ = fmt.Fprintf(stdout, "%s: %s (%s), algorithm %s\n", st.Service, st.Reason, st.Message, st.Algorithm)
		for _, u := range st.Upstreams {
			line := fmt.Sprintf("  %-28s %-10s weight=%d active=%d fails=%d", u.Dial, u.State, u.Weight, u.ActiveConns, u.Fails)
			if u.Reason != "" {
				line += "  " + u.Reason
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
	})
}

func runLBExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb export", "print the artifact as JSON to stdout and nothing else", stderr)
	format := fs.String("format", "", "terraform, cdk, cloudformation, caddy or caddy-json (required)")
	outFile := fs.String("out", "", "write the generated file here instead of stdout")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb export <app> --format FORMAT [--out FILE] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "lb export", "app name")
	if !ok {
		return exitUsage
	}
	if *format == "" {
		_, _ = fmt.Fprintf(stderr, "%s: lb export requires --format\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	art, err := client.ExportLoadBalancer(context.Background(), name, *format)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export load balancer for app %q: %w", name, err))
	}
	for _, w := range art.Warnings {
		_, _ = fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	if *outFile != "" {
		if err := os.WriteFile(*outFile, []byte(art.Body), 0o600); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", *outFile, err))
		}
		_, _ = fmt.Fprintf(stderr, "wrote %s\n", *outFile)
		return exitOK
	}
	return writeScheduledTaskResult(stdout, stderr, of, art, func() { _, _ = fmt.Fprint(stdout, art.Body) })
}

func runLBImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb import", "print the resulting config as JSON to stdout and nothing else", stderr)
	file := fs.String("file", "", "app.yaml to read the loadbalancer: block from (required)")
	service := fs.String("service", "", "service inside app.yaml (default: the app name)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb import <app> --file app.yaml [--service S] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "lb import", "app name")
	if !ok {
		return exitUsage
	}
	if *file == "" {
		_, _ = fmt.Fprintf(stderr, "%s: lb import requires --file\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	data, err := os.ReadFile(*file) //nolint:gosec // operator-supplied path
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read %s: %w", *file, err))
	}
	parsed, err := spec.Parse(data)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	svc := *service
	if svc == "" {
		svc = name
	}
	cfg, err := parsed.LoadBalancerFor(svc)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if cfg == nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("service %q in %s has no loadbalancer block", svc, *file))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetLoadBalancer(context.Background(), name, *cfg)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("import load balancer for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printLBHuman(stdout, res) })
}

func parseWeights(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("weights must be comma separated integers, got %q", p)
		}
		out[i] = n
	}
	return out, nil
}

func runLBSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb set", "print the resulting config as JSON to stdout and nothing else", stderr)
	algorithm := fs.String("algorithm", "", "round_robin, least_conn, ip_hash, uri_hash, cookie or weighted")
	cookie := fs.String("cookie-name", "", "sticky session cookie name (algorithm cookie)")
	weights := fs.String("weights", "", "comma separated weight per replica, e.g. 3,1 (algorithm weighted)")
	healthPath := fs.String("health-path", "", "active health check path, e.g. /healthz")
	healthInterval := fs.String("health-interval", "", "active health check interval, e.g. 5s")
	healthTimeout := fs.String("health-timeout", "", "active health check timeout, e.g. 2s")
	healthPasses := fs.Int("health-passes", 0, "consecutive passes to mark healthy")
	healthFails := fs.Int("health-fails", 0, "consecutive failures to mark unhealthy")
	healthStatus := fs.Int("health-status", 0, "expected status code (default any 2xx or 3xx)")
	maxFails := fs.Int("max-fails", 0, "passive health: failures before an upstream is skipped")
	failDuration := fs.String("fail-duration", "", "passive health: how long failures are remembered, e.g. 30s")
	retries := fs.Int("retries", 0, "retry a failed request on another upstream this many times")
	tryDuration := fs.String("try-duration", "", "give up retrying after this long, e.g. 5s")
	slowStart := fs.String("slow-start", "", "ramp a new upstream's weight over this long, e.g. 30s (weighted only)")
	drain := fs.String("drain-timeout", "", "keep in-flight streams open this long during a cutover, e.g. 15s")
	reqTimeout := fs.String("request-timeout", "", "upstream response header timeout, e.g. 30s")
	rps := fs.Int("rate-limit-rps", 0, "requests per second per client")
	burst := fs.Int("rate-limit-burst", 0, "extra burst allowance per client")
	upstreamTLS := fs.Bool("upstream-tls", false, "speak HTTPS to upstreams")
	tlsInsecure := fs.Bool("upstream-tls-insecure", false, "skip upstream certificate verification")
	tlsName := fs.String("upstream-tls-server-name", "", "expected upstream certificate name")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb set <app> [flags]\n\nOnly the flags you pass change; everything else keeps its current value.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "lb set", "app name")
	if !ok {
		return exitUsage
	}
	w, err := parseWeights(*weights)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}

	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	current, err := client.GetLoadBalancer(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get load balancer for app %q: %w", name, err))
	}
	cfg := apiclient.LoadBalancerConfig{}
	if current.Config != nil {
		cfg = *current.Config
	}
	applyLBFlags(&cfg, set, lbFlagValues{
		algorithm: *algorithm, cookie: *cookie, weights: w,
		healthPath: *healthPath, healthInterval: *healthInterval, healthTimeout: *healthTimeout,
		healthPasses: *healthPasses, healthFails: *healthFails, healthStatus: *healthStatus,
		maxFails: *maxFails, failDuration: *failDuration, retries: *retries, tryDuration: *tryDuration,
		slowStart: *slowStart, drain: *drain, reqTimeout: *reqTimeout, rps: *rps, burst: *burst,
		upstreamTLS: *upstreamTLS, tlsInsecure: *tlsInsecure, tlsName: *tlsName,
	})

	res, err := client.SetLoadBalancer(context.Background(), name, cfg)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set load balancer for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printLBHuman(stdout, res) })
}
