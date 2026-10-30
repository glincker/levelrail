package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/bench"
)

type benchReport struct {
	Target      string             `json:"target"`
	Rate        float64            `json:"rate_rps"`
	DurationS   float64            `json:"duration_s"`
	Hardware    string             `json:"hardware,omitempty"`
	Client      benchClientInfo    `json:"client"`
	Ingress     bench.Stats        `json:"ingress"`
	Direct      *bench.Stats       `json:"direct,omitempty"`
	Overhead    *bench.Overhead    `json:"ingress_overhead,omitempty"`
	Event       *bench.EventReport `json:"event,omitempty"`
	StillServed *bool              `json:"ingress_kept_serving,omitempty"`
}

type benchClientInfo struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	CPUs int    `json:"cpus"`
	Go   string `json:"go"`
}

// runBench implements "bench run": an open-loop load run against an app
// URL, optionally with one failure scenario fired mid-run.
func runBench(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "chaos":
			return runBenchChaos(prog, args[1:], stdout, stderr, lookupEnv)
		case "soak":
			return runBenchSoak(prog, args[1:], stdout, stderr, lookupEnv)
		case "results":
			return runBenchResults(prog, args[1:], stdout, stderr, lookupEnv)
		}
	}
	if len(args) == 0 || args[0] != "run" {
		_, _ = fmt.Fprint(stderr, benchUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bench run", "print the report as JSON to stdout and nothing else", stderr)
	url := fs.String("url", "", "app URL served through the ingress (required)")
	direct := fs.String("direct-url", "", "URL of one replica reached without the ingress, to measure ingress overhead")
	rate := fs.Float64("rate", 100, "constant arrival rate in requests per second")
	duration := fs.Duration("duration", 30*time.Second, "length of the measured run")
	baseline := fs.Duration("baseline-duration", 10*time.Second, "length of the direct run used for the overhead comparison")
	timeout := fs.Duration("timeout", 5*time.Second, "per-request timeout, a timeout counts as an error")
	eventAt := fs.Duration("event-at", 0, "when the scenario fires (default one third into the run)")
	kill := fs.String("kill-replica-url", "", "failover: POST this replica URL (fixture /__bench/die) mid-run")
	drain := fs.String("drain-node", "", "failover: drain this node ID mid-run (needs --drain-target)")
	drainTarget := fs.String("drain-target", "", "node ID that receives the drained node's apps")
	restart := fs.Bool("restart-control-plane", false, "restart the control plane mid-run and report whether ingress kept serving")
	hardware := fs.String("hardware", "", "free-form server hardware note, kept in the report (CPU, RAM, kernel, Docker, node count)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, benchUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if *url == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--url is required"))
	}
	if *rate <= 0 || *duration <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--rate and --duration must be positive"))
	}
	scenarios := 0
	for _, set := range []bool{*kill != "", *drain != "", *restart} {
		if set {
			scenarios++
		}
	}
	if scenarios > 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("choose at most one of --kill-replica-url, --drain-node, --restart-control-plane"))
	}
	if *drain != "" && *drainTarget == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--drain-node needs --drain-target"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	httpc := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 512}}

	var ev *bench.Event
	at := *eventAt
	if at <= 0 {
		at = *duration / 3
	}
	switch {
	case *kill != "":
		ev = &bench.Event{Name: "kill-replica", At: at, Fire: func(c context.Context) error { return postNoBody(c, httpc, *kill) }}
	case *drain != "":
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		ev = &bench.Event{Name: "drain-node", At: at, Fire: func(c context.Context) error {
			_, err := client.DrainNode(c, *drain, *drainTarget)
			return err
		}}
	case *restart:
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		ev = &bench.Event{Name: "restart-control-plane", At: at, Fire: func(c context.Context) error {
			_, err := client.RestartControlPlane(c)
			return err
		}}
	}

	rep := benchReport{
		Target: *url, Rate: *rate, DurationS: duration.Seconds(), Hardware: *hardware,
		Client: benchClientInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Go: runtime.Version()},
	}

	if *direct != "" {
		_, _ = fmt.Fprintln(stderr, "measuring the direct replica baseline...")
		d, err := bench.Run(ctx, httpc, bench.Config{URL: *direct, Rate: *rate, Duration: *baseline, Timeout: *timeout}, nil)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("direct baseline: %w", err))
		}
		rep.Direct = &d.Stats
	}

	_, _ = fmt.Fprintln(stderr, "measuring through the ingress...")
	res, err := bench.Run(ctx, httpc, bench.Config{URL: *url, Rate: *rate, Duration: *duration, Timeout: *timeout}, ev)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("load run: %w", err))
	}
	rep.Ingress = res.Stats
	rep.Event = res.Event
	if rep.Direct != nil {
		o := bench.Compare(res.Stats, *rep.Direct)
		rep.Overhead = &o
	}
	if *restart && res.Event != nil && res.Event.FireError == "" {
		served := res.Event.Dropped == 0
		rep.StillServed = &served
	}

	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printBenchHuman(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func postNoBody(ctx context.Context, c *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func printBenchHuman(out io.Writer, r benchReport) {
	row := func(label string, s bench.Stats) {
		_, _ = fmt.Fprintf(out, "%-8s %9.1f %7.2f%% %8.2f %8.2f %8.2f %8.2f\n", label, s.AchievedRPS, s.ErrorRate*100, s.P50MS, s.P95MS, s.P99MS, s.MaxMS)
	}
	_, _ = fmt.Fprintf(out, "target %s at %.0f req/s for %.0fs\n\n", r.Target, r.Rate, r.DurationS)
	_, _ = fmt.Fprintf(out, "%-8s %9s %8s %8s %8s %8s %8s\n", "path", "rps", "errors", "p50 ms", "p95 ms", "p99 ms", "max ms")
	row("ingress", r.Ingress)
	if r.Direct != nil {
		row("direct", *r.Direct)
	}
	if r.Overhead != nil {
		_, _ = fmt.Fprintf(out, "\ningress overhead: p50 %+.2f ms, p95 %+.2f ms, p99 %+.2f ms\n", r.Overhead.P50MS, r.Overhead.P95MS, r.Overhead.P99MS)
	}
	if r.Ingress.Saturated > 0 {
		_, _ = fmt.Fprintf(out, "\nwarning: %d requests were shed by the client in-flight cap, the load generator was the bottleneck\n", r.Ingress.Saturated)
	}
	if e := r.Event; e != nil {
		_, _ = fmt.Fprintf(out, "\nscenario %s fired at %.0f ms\n", e.Name, e.FiredAtMS)
		if e.FireError != "" {
			_, _ = fmt.Fprintf(out, "  scenario action failed: %s\n", e.FireError)
		}
		_, _ = fmt.Fprintf(out, "  dropped requests: %d\n  error window:     %.0f ms\n  recovery:         %.0f ms after the event (recovered: %t)\n", e.Dropped, e.ErrorWindowMS, e.RecoveryMS, e.Recovered)
		if r.StillServed != nil {
			_, _ = fmt.Fprintf(out, "  ingress kept serving: %t\n", *r.StillServed)
		}
	}
}

func benchUsage(prog string) string {
	return fmt.Sprintf(`Usage: %[1]s bench run --url URL [flags]

Open-loop load run (constant arrival rate, latency measured from the intended
send time) against an app served through the ingress.

  --url URL                 app URL through the ingress (required)
  --direct-url URL          one replica without the ingress, for the overhead comparison
  --rate N                  requests per second (default 100)
  --duration D              measured run length (default 30s)
  --baseline-duration D     direct run length (default 10s)
  --timeout D               per-request timeout (default 5s)
  --event-at D              when the scenario fires (default one third into the run)

One failure scenario at most:
  --kill-replica-url URL    POST a replica's fixture /__bench/die mid-run
  --drain-node ID           drain a node mid-run, with --drain-target ID
  --restart-control-plane   restart the control plane mid-run

  --hardware TEXT           server hardware note stored in the report
  --json                    print the report as JSON to stdout and nothing else

Related: "bench chaos" runs the failure scenarios, "bench results" lists and
compares stored runs, "bench soak" holds load and samples the footprint.
`, prog)
}
