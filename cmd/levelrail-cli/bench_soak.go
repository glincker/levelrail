package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/soak"
)

const defaultSoakPaths = "/api/v1/apps,/api/v1/changelog,/api/v1/system/control-plane-health"

type soakReport struct {
	Duration string      `json:"duration"`
	CSV      string      `json:"csv,omitempty"`
	Result   soak.Result `json:"result"`
}

// runBenchSoak implements "bench soak": read churn against a control plane
// (a demo instance by default) while sampling RSS, goroutines, file
// descriptors and WAL size to a CSV, then judging growth against limits.
func runBenchSoak(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bench soak", "print the report as JSON to stdout and nothing else", stderr)
	duration := fs.Duration("duration", 10*time.Minute, "how long to churn")
	interval := fs.Duration("sample-interval", 15*time.Second, "time between samples")
	workers := fs.Int("concurrency", 4, "parallel churn workers")
	rate := fs.Float64("rate", 20, "total churn requests per second")
	paths := fs.String("paths", defaultSoakPaths, "comma-separated GET paths to churn")
	csvPath := fs.String("csv", "", "write the samples to this CSV file")
	ratchetPath := fs.String("ratchet", "", "JSON file holding the last passing run's end state; the run fails when it ends worse")
	record := fs.Bool("record", false, "with --ratchet: overwrite the file when the run passes")
	slack := fs.Float64("ratchet-slack-pct", 10, "tolerated growth over the ratchet, in percent")
	rssPct := fs.Float64("max-rss-growth-pct", 25, "fail when RSS grows more than this between start and end (0 disables)")
	grGrowth := fs.Int("max-goroutine-growth", 100, "fail when goroutines grow by more than this (0 disables)")
	fdGrowth := fs.Int("max-fd-growth", 50, "fail when open file descriptors grow by more than this (0 disables)")
	walMax := fs.Int64("max-wal-bytes", 256<<20, "fail when the SQLite WAL exceeds this size (0 disables)")
	errPct := fs.Float64("max-error-rate-pct", 1, "fail when churn errors exceed this percent (0 disables)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, benchSoakUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if *duration <= 0 || *interval <= 0 || *workers <= 0 || *rate <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--duration, --sample-interval, --concurrency and --rate must be positive"))
	}
	var targets []string
	for _, p := range strings.Split(*paths, ",") {
		if p = strings.TrimSpace(p); strings.HasPrefix(p, "/api/") {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--paths needs at least one path starting with /api/"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runCtx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()

	var requests, errs atomic.Int64
	var wg sync.WaitGroup
	perWorker := time.Duration(float64(time.Second) * float64(*workers) / *rate)
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			t := time.NewTicker(perWorker)
			defer t.Stop()
			for i := offset; ; i++ {
				select {
				case <-runCtx.Done():
					return
				case <-t.C:
				}
				requests.Add(1)
				if err := client.Probe(runCtx, targets[i%len(targets)]); err != nil && runCtx.Err() == nil {
					errs.Add(1)
				}
			}
		}(w)
	}

	var samples []soak.Sample
	take := func() error {
		h, err := client.GetCPHealth(ctx)
		if err != nil {
			return err
		}
		samples = append(samples, soak.Sample{
			At: time.Now(), RSSBytes: h.Footprint.RSSBytes, Goroutines: h.Runtime.Goroutines,
			OpenFDs: h.Footprint.OpenFDs, WALBytes: h.DB.WALBytes, Requests: requests.Load(), Errors: errs.Load(),
		})
		return nil
	}
	if err := take(); err != nil {
		cancel()
		wg.Wait()
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read control plane health: %w", err))
	}
	tick := time.NewTicker(*interval)
	defer tick.Stop()
	var sampleErr error
sampling:
	for {
		select {
		case <-runCtx.Done():
			break sampling
		case <-tick.C:
			if err := take(); err != nil && sampleErr == nil {
				sampleErr = err
			}
		}
	}
	wg.Wait()
	if err := take(); err != nil && sampleErr == nil {
		sampleErr = err
	}

	res := soak.Evaluate(samples, soak.Limits{RSSGrowthPct: *rssPct, GoroutineGrowth: *grGrowth, FDGrowth: *fdGrowth, WALMaxBytes: *walMax, ErrorRatePct: *errPct})
	if sampleErr != nil {
		res.Pass = false
		res.Violations = append(res.Violations, "a health sample failed: "+sampleErr.Error())
	}
	if *ratchetPath != "" {
		prev, found, err := soak.LoadRatchet(*ratchetPath)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		if found {
			soak.CheckRatchet(&res, prev, *slack)
		}
		if res.Pass && (*record || !found) {
			if err := soak.SaveRatchet(*ratchetPath, res.Summary); err != nil {
				return reportError(stdout, stderr, jsonOut, err)
			}
		}
	}
	if *csvPath != "" {
		f, err := os.Create(*csvPath) //nolint:gosec // operator-supplied output path for a local CLI flag
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("create csv: %w", err))
		}
		werr := soak.WriteCSV(f, samples)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return reportError(stdout, stderr, jsonOut, werr)
		}
	}

	rep := soakReport{Duration: duration.String(), CSV: *csvPath, Result: res}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printSoak(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !res.Pass {
		return exitCheckFailed
	}
	return exitOK
}

func printSoak(out io.Writer, r soakReport) {
	s := r.Result.Summary
	_, _ = fmt.Fprintf(out, "soak %s, %d samples, %d requests (%d errors)\n", r.Duration, s.Samples, s.Requests, s.Errors)
	_, _ = fmt.Fprintf(out, "rss        %d -> %d bytes\ngoroutines %d -> %d\nopen fds   %d -> %d\nmax wal    %d bytes\n", s.StartRSSBytes, s.EndRSSBytes, s.StartGoroutines, s.EndGoroutines, s.StartOpenFDs, s.EndOpenFDs, s.MaxWALBytes)
	if r.Result.Pass {
		_, _ = fmt.Fprintln(out, "PASS")
		return
	}
	for _, v := range r.Result.Violations {
		_, _ = fmt.Fprintf(out, "FAIL: %s\n", v)
	}
}

func benchSoakUsage(prog string) string {
	return fmt.Sprintf(`Usage: %[1]s bench soak [flags]

Churns read traffic against a control plane (point it at a demo instance for a
nightly run) and samples RSS, goroutines, open file descriptors and WAL size.
Fails when growth between the start and end of the run exceeds the limits, or
when the end state is worse than the --ratchet file.

  --duration D              how long to churn (default 10m)
  --sample-interval D       time between samples (default 15s)
  --concurrency N           parallel workers (default 4)
  --rate N                  total requests per second (default 20)
  --paths LIST              comma-separated GET paths under /api/
  --csv FILE                write the samples to a CSV file
  --ratchet FILE            end state of the last passing run; --record updates it
  --ratchet-slack-pct N     tolerated growth over the ratchet (default 10)
  --max-rss-growth-pct N, --max-goroutine-growth N, --max-fd-growth N,
  --max-wal-bytes N, --max-error-rate-pct N    limits, 0 disables one
  --json                    print the report as JSON to stdout and nothing else

Exit code 0 passes, 1 fails a limit.
`, prog)
}
