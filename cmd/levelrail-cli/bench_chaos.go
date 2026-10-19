package main

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/chaos"
)

type chaosRunFunc func(ctx context.Context, scenario, hardwareNote string) chaos.Result

// runBenchChaos implements "bench chaos list|run|report".
func runBenchChaos(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, benchChaosUsage(prog))
		return exitUsage
	}
	sub := args[0]
	if sub != "list" && sub != "run" && sub != "report" && sub != "disk" && sub != "free-space" {
		_, _ = fmt.Fprint(stderr, benchChaosUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bench chaos "+sub, "print the result as JSON to stdout and nothing else", stderr)
	record := fs.Bool("record", false, "run: store the results on the control plane so the resilience report shows them")
	hardware := fs.String("hardware", "", "run: free-form hardware note kept in each result")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, benchChaosUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	switch sub {
	case "list":
		info := chaos.Catalog()
		return finishChaos(stdout, stderr, of, info, func() {
			for _, i := range info {
				_, _ = fmt.Fprintf(stdout, "%-24s %s\n    expects: %s\n", i.Name, i.Title, i.Expectation)
			}
		})
	case "disk":
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		d, err := client.GetDiskResilience(context.Background())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, d, func() { writeDiskTable(stdout, d) })
	case "free-space":
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		r, err := client.FreeDiskSpace(context.Background())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, r, func() {
			_, _ = fmt.Fprintf(stdout, "reclaimed %d MiB\n", r.ReclaimedBytes>>20)
			writeDiskTable(stdout, r.Disk)
		})
	case "report":
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		rep, err := client.GetResilienceReport(context.Background())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishChaos(stdout, stderr, of, rep, func() {
			chaos.WriteTable(stdout, rep.Latest)
			for _, n := range rep.Unmeasured {
				_, _ = fmt.Fprintf(stdout, "%-24s not yet measured\n", n)
			}
		})
	}

	names := fs.Args()
	if len(names) != 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("name one scenario or all, see: %s bench chaos list", prog))
	}
	var scenarios []string
	if names[0] == "all" {
		for _, i := range chaos.Catalog() {
			scenarios = append(scenarios, i.Name)
		}
	} else if chaos.Valid(names[0]) {
		scenarios = []string{names[0]}
	} else {
		return reportError(stdout, stderr, jsonOut, newValidationError("unknown scenario %s, see: %s bench chaos list", names[0], prog))
	}
	runner := liveChaosRunner()
	if runner == nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("this build has no live chaos scenarios: build with -tags chaoslive (needs a local Docker daemon)"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var results []chaos.Result
	for _, s := range scenarios {
		_, _ = fmt.Fprintf(stderr, "running %s...\n", s)
		results = append(results, runner(ctx, s, *hardware))
		if ctx.Err() != nil {
			break
		}
	}
	hw := ""
	if len(results) > 0 {
		hw = results[0].Hardware
	}
	summary := chaos.Summarize(hw, results)
	if *record {
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		if _, err := client.RecordChaosResults(context.Background(), results); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("record results: %w", err))
		}
	}
	code := finishChaos(stdout, stderr, of, summary, func() { chaos.WriteTable(stdout, results) })
	if code == exitOK && summary.Failed > 0 {
		return exitUsage
	}
	return code
}

func finishChaos(stdout, stderr io.Writer, of outputFlags, data any, table func()) int {
	if err := renderResult(stdout, of.Format, of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func writeDiskTable(w io.Writer, d apiclient.DiskResilience) {
	for _, v := range d.Volumes {
		_, _ = fmt.Fprintf(w, "%-8s %-9s %6d MiB free of %d MiB  %s\n", v.Name, v.Level, v.FreeBytes>>20, v.TotalBytes>>20, v.Path)
	}
}

func benchChaosUsage(prog string) string {
	return fmt.Sprintf(`Usage: %[1]s bench chaos <list|run|report|disk|free-space> [flags]

Failure scenarios measured against throwaway local Docker containers (unique
name prefix, always removed afterwards).

  list                 the scenarios and the behaviour each one asserts
  run <scenario|all>   run live (needs a build with -tags chaoslive and a local
                       Docker daemon); prints a table, or JSON with --json
  report               the latest recorded results from the control plane
  disk                 free space on the data and Docker volumes, and whether risky operations are refused
  free-space           run a forced cleanup pass now (the fix for a low-disk refusal), needs a root token

  --record             run: store the results on the control plane
  --hardware TEXT      run: hardware note kept in each result
  --json               print machine-readable output only

Exit code 2 when any scenario assertion failed.
`, prog)
}
