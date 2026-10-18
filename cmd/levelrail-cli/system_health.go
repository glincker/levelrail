package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runSystemHealth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system health", "print control plane health as JSON to stdout and nothing else", stderr)
	fix := fs.String("fix", "", "apply a fix: wal_checkpoint, db_optimize or reset_slow_log")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, systemUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		h   apiclient.CPHealth
		err error
	)
	if *fix != "" {
		h, err = client.FixCPHealth(ctx, *fix)
	} else {
		h, err = client.GetCPHealth(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("control plane health: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, h, func() { printCPHealth(stdout, h) })
}

func printCPHealth(out io.Writer, h apiclient.CPHealth) {
	_, _ = fmt.Fprintf(out, "status:      %s\n", h.Status)
	_, _ = fmt.Fprintf(out, "goroutines:  %d\nheap:        %d MiB\n", h.Runtime.Goroutines, h.Runtime.HeapBytes>>20)
	_, _ = fmt.Fprintf(out, "db latency:  p50 %.1f ms, p95 %.1f ms, p99 %.1f ms\n", h.DB.P50MS, h.DB.P95MS, h.DB.P99MS)
	_, _ = fmt.Fprintf(out, "db queue:    %d waits, %.0f ms total\nwal:         %d MiB of %d MiB db\n", h.DB.WaitCount, h.DB.WaitMS, h.DB.WALBytes>>20, h.DB.DBBytes>>20)
	_, _ = fmt.Fprintf(out, "reconcile:   %d controllers, last pass %.0f ms, p95 %.0f ms, %.0f s since last pass\n", h.Reconcile.Controllers, h.Reconcile.LastPassMS, h.Reconcile.P95MS, h.Reconcile.LagSeconds)
	_, _ = fmt.Fprintf(out, "streams:     %d of %d\n", h.Streams.Active, h.Streams.Cap)
	for _, f := range h.Findings {
		line := fmt.Sprintf("  [%s] %s: %.1f (limit %.1f)", f.Severity, f.ID, f.Value, f.Threshold)
		if f.Fix != "" {
			line += fmt.Sprintf("  fix: system health --fix %s", f.Fix)
		}
		_, _ = fmt.Fprintln(out, line)
	}
}
