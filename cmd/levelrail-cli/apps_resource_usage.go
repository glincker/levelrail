package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
)

// runAppsResourceUsage implements "apps resource-usage": GET
// /api/v1/apps/resource-usage (internal/api/app_resource_usage.go), the
// same ranking the dashboard's "top resource consumers" panel reads.
// Sorted by CPU descending by default since that's the common "what's
// hot right now" question; --json preserves the server's own name
// ordering for scripting.
func runAppsResourceUsage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps resource-usage", "print resource usage as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps resource-usage [flags]\n\nLists every app's latest known CPU, memory, and network usage, ranked by\nCPU descending, the same ranking the dashboard shows.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	usage, err := client.ListAppResourceUsage(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list app resource usage: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, usage, func() { printAppResourceUsageTable(stdout, usage) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAppResourceUsageTable(out io.Writer, usage []appResourceUsageResource) {
	if len(usage) == 0 {
		_, _ = fmt.Fprintln(out, "no apps")
		return
	}
	ranked := make([]appResourceUsageResource, len(usage))
	copy(ranked, usage)
	sort.Slice(ranked, func(i, j int) bool {
		return cpuPercentOrZero(ranked[i]) > cpuPercentOrZero(ranked[j])
	})

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tCPU\tMEMORY\tNET RX\tNET TX")
	for _, u := range ranked {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			u.Name, formatCPUPercent(u.CPUPercent), formatMemoryUsage(u.MemoryUsageBytes, u.MemoryLimitBytes),
			formatByteRate(u.NetworkRxBytes), formatByteRate(u.NetworkTxBytes))
	}
	_ = tw.Flush()
}

func cpuPercentOrZero(u appResourceUsageResource) float64 {
	if u.CPUPercent == nil {
		return 0
	}
	return *u.CPUPercent
}

func formatCPUPercent(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", *v)
}

func formatMemoryUsage(usage, limit *float64) string {
	if usage == nil {
		return "-"
	}
	used := formatRecommendationBytes(int64(*usage))
	if limit == nil || *limit <= 0 {
		return used
	}
	return fmt.Sprintf("%s / %s", used, formatRecommendationBytes(int64(*limit)))
}

func formatByteRate(v *float64) string {
	if v == nil {
		return "-"
	}
	return formatRecommendationBytes(int64(*v))
}
