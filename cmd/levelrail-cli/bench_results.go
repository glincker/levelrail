package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/benchreport"
)

func uploadBenchReport(ctx context.Context, client *Client, rep benchreport.Report, stderr io.Writer) error {
	b, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("encode results: %w", err)
	}
	s, err := client.UploadBenchmark(ctx, b)
	if err != nil {
		return fmt.Errorf("upload results: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "uploaded as %s\n", s.ID)
	return nil
}

// runBenchResults implements "bench results list|show|compare|upload|delete" over the stored results files.
func runBenchResults(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, benchResultsUsage(prog))
		return exitUsage
	}
	sub := args[0]
	if !strings.Contains(" list show compare upload delete ", " "+sub+" ") {
		_, _ = fmt.Fprint(stderr, benchResultsUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bench results "+sub, "print the result as JSON to stdout and nothing else", stderr)
	limit := fs.Int("limit", 0, "list: show at most this many runs")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, benchResultsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	need := map[string]int{"list": 0, "show": 1, "compare": 2, "upload": 1, "delete": 1}[sub]
	if len(rest) != need {
		return reportError(stdout, stderr, jsonOut, newValidationError("bench results %s takes %d argument(s), see: %s bench results", sub, need, prog))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	switch sub {
	case "list":
		l, err := client.ListBenchmarks(ctx, *limit)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, l, func() { writeBenchRuns(stdout, l.Runs) })
	case "show":
		r, err := client.GetBenchmark(ctx, rest[0])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, r, func() { _, _ = io.WriteString(stdout, benchreport.Markdown(r.Report)) })
	case "compare":
		c, err := client.CompareBenchmarks(ctx, rest[0], rest[1])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, c, func() { writeBenchComparison(stdout, c) })
	case "upload":
		b, err := os.ReadFile(rest[0]) //nolint:gosec // operator supplied results file
		if err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("read results file: %v", err))
		}
		s, err := client.UploadBenchmark(ctx, b)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, s, func() { _, _ = fmt.Fprintf(stdout, "stored as %s\n", s.ID) })
	default:
		if err := client.DeleteBenchmark(ctx, rest[0]); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, map[string]string{"deleted": rest[0]}, func() { _, _ = fmt.Fprintf(stdout, "deleted %s\n", rest[0]) })
	}
}

func writeBenchRuns(w io.Writer, runs []apiclient.BenchmarkRunSummary) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tWHEN\tVERSION\tCOMMIT\tHOST\tPASSED\tFAILED\tRUNS")
	for _, r := range runs {
		commit := r.Commit
		if len(commit) > 10 {
			commit = commit[:10]
		}
		if r.Dirty {
			commit += "+dirty"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\n", r.ID, r.GeneratedAt.Format("2006-01-02 15:04"), r.Version, commit, r.HostSummary, r.Passed, r.Failed, r.Repeat)
	}
	_ = tw.Flush()
}

func writeBenchComparison(w io.Writer, c benchreport.Comparison) {
	_, _ = fmt.Fprintf(w, "base %s (%s, %s)\nhead %s (%s, %s)\nvalues: %s, changes under %.0f%% read as same\n\n", c.Base.ID, c.Base.Version, c.Base.Commit, c.Head.ID, c.Head.Version, c.Head.Commit, c.Statistic, c.NoisePct)
	for _, warn := range c.Warnings {
		_, _ = fmt.Fprintf(w, "WARNING %s\n", warn)
	}
	if len(c.Warnings) > 0 {
		_, _ = fmt.Fprintln(w)
	}
	for _, s := range c.Status {
		_, _ = fmt.Fprintf(w, "status %-24s %s -> %s\n", s.Scenario, s.Base, s.Head)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SCENARIO\tMETRIC\tBASE\tHEAD\tDELTA\tPCT\tVERDICT")
	for _, d := range c.Deltas {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", d.Scenario, d.Metric, fnum(d.Base), fnum(d.Head), fnum(d.Delta), fpct(d.DeltaPct), d.Direction)
	}
	_ = tw.Flush()
}

func fnum(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", *v)
}

func fpct(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%+.1f%%", *v)
}

func benchResultsUsage(prog string) string {
	return fmt.Sprintf(`Usage: %[1]s bench results <list|show|compare|upload|delete> [flags]

Benchmark results files stored on the control plane, produced by
"bench chaos run --out FILE" (or uploaded directly with --upload).

  list                     stored runs, newest first (--limit N)
  show ID                  one run: host, build, caveats, per-scenario numbers, limits table
  compare BASE HEAD        every metric of two runs with deltas (median over runs on each side)
  upload FILE              store a results file
  delete ID                remove a run (needs a root token)

Results from a non-Linux host are never Linux production numbers; compare says so.
  --json                   print machine-readable output only
`, prog)
}
