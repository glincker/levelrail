package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsCostEstimate implements "apps cost <name>": GET
// /api/v1/apps/{name}/cost-estimate (internal/api/cost_estimate.go's
// handleAppCostEstimate), a read-only, deterministic "what this would
// cost elsewhere" estimate derived from the app's declared or observed
// CPU/memory. Not a real bill: the same read-and-suggest layer
// runAppsResourceRecommendation already establishes.
func runAppsCostEstimate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps cost", "print the estimate as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps cost <name> [flags]\n\nEstimates what this app's declared (or, failing that, observed)\nCPU/memory allocation would cost per month under a few illustrative\nreference pricing providers. This is an ESTIMATE, not a real bill;\nsee docs/cost-estimate.md for the formula and how to correct the\nrates for your own region with APP_COST_ESTIMATE_* env vars.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps cost", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	est, err := client.GetAppCostEstimate(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get cost estimate for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, est, func() { printCostEstimateHuman(stdout, name, est) })
}

func printCostEstimateHuman(out io.Writer, name string, est costEstimateResource) {
	_, _ = fmt.Fprintf(out, "app: %s\n", name)
	_, _ = fmt.Fprintf(out, "size: %.2f vCPU (%s), %.2f GiB memory (%s)\n", est.VCPUCores, est.CPUBasis, est.MemoryGiB, est.MemoryBasis)
	_, _ = fmt.Fprintln(out, "\nestimated monthly cost by reference provider:")
	for _, p := range est.Providers {
		floor := ""
		if p.MinimumApplied {
			floor = " (provider minimum)"
		}
		_, _ = fmt.Fprintf(out, "  %-32s $%.2f/mo%s\n", p.Label, p.TotalUSD, floor)
		_, _ = fmt.Fprintf(out, "    cpu $%.2f + memory $%.2f\n", p.CPUCostUSD, p.MemoryCostUSD)
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", est.Note)
}
