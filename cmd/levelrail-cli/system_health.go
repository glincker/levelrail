package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runSystemHealth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system health", "print control plane health as JSON to stdout and nothing else", stderr)
	fix := fs.String("fix", "", "apply a fix: wal_checkpoint, db_optimize, reset_slow_log, telemetry_prune, reset_reconcile_guard, integrity_check or integrity_restore")
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
	fp := h.Footprint
	_, _ = fmt.Fprintf(out, "footprint:   rss %d MiB, %d threads, cpu %.1f%%, metrics db %d MiB, memory limit %d MiB (%s)\n", fp.RSSBytes>>20, fp.Threads, fp.CPUPercent, fp.TelemetryBytes>>20, fp.MemoryLimit.LimitBytes>>20, fp.MemoryLimit.Source)
	for _, l := range fp.Loops {
		_, _ = fmt.Fprintf(out, "  loop %-28s every %6.0fs runs %-6d last %6.1f ms cost %.3f%%\n", l.Name, l.IntervalSecs, l.Runs, l.LastDuration, l.CostPercent)
	}
	g := h.Reconcile.Guard
	if g.PassesDeferred > 0 || len(g.Breakers) > 0 {
		_, _ = fmt.Fprintf(out, "protection:  %d passes deferred, %d limited in the last 5 min, %d breakers open\n", g.PassesDeferred, g.RecentLimited, g.OpenBreakers)
		for _, b := range g.Breakers {
			_, _ = fmt.Fprintf(out, "  breaker %-28s %-9s failures %d trips %d panics %d %s\n", b.Name, b.State, b.Failures, b.Trips, b.Panics, b.LastError)
		}
	}
	for _, f := range h.Findings {
		line := fmt.Sprintf("  [%s] %s: %.1f (limit %.1f)", f.Severity, f.ID, f.Value, f.Threshold)
		if f.Fix != "" {
			line += fmt.Sprintf("  fix: system health --fix %s", f.Fix)
		}
		_, _ = fmt.Fprintln(out, line)
	}
}
