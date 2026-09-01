package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsResourceRecommendation implements "apps resource-recommendation
// <name>": GET /api/v1/apps/{name}/resource-recommendation
// (internal/api/resource_recommendation.go's own
// handleAppResourceRecommendation), a read-only, deterministic memory/CPU
// right-sizing suggestion derived from the app's own historical usage.
// Never writes anything and never applies a suggestion; the same
// read-and-suggest layer runAppsDiagnose already establishes.
func runAppsResourceRecommendation(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps resource-recommendation", "print the recommendation as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps resource-recommendation <name> [flags]\n\nSuggests memory and CPU limits for an app based on its own historical\nusage (p95/p99 over a lookback window) and current limits, using a\ndeterministic engine, never an external model. Never changes anything;\nthe operator decides whether to act on the suggestion.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps resource-recommendation", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	rec, err := client.GetAppResourceRecommendation(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get resource recommendation for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, rec, func() { printResourceRecommendationHuman(stdout, "app", rec) })
}
