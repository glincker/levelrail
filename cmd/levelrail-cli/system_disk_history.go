package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSystemDiskHistory implements "system disk history [--days N] [--rows N]"
// (internal/api/disk_history.go).
func runSystemDiskHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "history" {
		_, _ = fmt.Fprint(stderr, systemUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system disk history", "print the history as JSON to stdout and nothing else", stderr)
	days := fs.Int("days", 0, "days of history to show (default and maximum: the retention setting)")
	rows := fs.Int("rows", 24, "most table rows to print, evenly spread over the window (JSON always has every sample)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s system disk history [--days N] [--rows N]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if *days < 0 || *rows < 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--days must not be negative and --rows must be at least 1"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetDiskHistory(context.Background(), *days)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disk history: %w", apiclient.NotSupported("disk history", err)))
	}
	return finishRender(stdout, stderr, of, res, func() { printDiskHistory(stdout, res, *rows) })
}

// thinSamples keeps at most limit entries spread evenly, always ending on the newest.
func thinSamples[T any](in []T, limit int) []T {
	if len(in) <= limit {
		return in
	}
	if limit == 1 {
		return in[len(in)-1:]
	}
	out := make([]T, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, in[i*(len(in)-1)/(limit-1)])
	}
	return out
}

func printDiskHistory(out io.Writer, r apiclient.DiskHistory, rows int) {
	if len(r.Samples) == 0 {
		_, _ = fmt.Fprintf(out, "no disk samples yet (retention %d days)\n", r.RetentionDays)
		return
	}
	switch {
	case r.Forecast.DaysUntilFull != nil:
		_, _ = fmt.Fprintf(out, "forecast: full in about %.0f days (from %d days of samples)\n\n", *r.Forecast.DaysUntilFull, r.Forecast.BasisDays)
	default:
		_, _ = fmt.Fprintf(out, "forecast: none (%d days of samples, not growing or too little data)\n\n", r.Forecast.BasisDays)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TIME\tFREE\tTOTAL\tIMAGES\tCONTAINERS\tVOLUMES\tBUILD CACHE")
	for _, s := range thinSamples(r.Samples, rows) {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.At.UTC().Format(time.DateTime),
			humanBytes(s.FreeBytes), humanBytes(s.TotalBytes), humanBytes(s.ImagesBytes),
			humanBytes(s.ContainersBytes), humanBytes(s.VolumesBytes), humanBytes(s.BuildCacheBytes))
	}
	_ = tw.Flush()
	if len(r.Samples) > rows {
		_, _ = fmt.Fprintf(out, "\nshowing %d of %d samples; use --json for all\n", min(rows, len(r.Samples)), len(r.Samples))
	}
}
