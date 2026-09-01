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
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps resource-recommendation", "print the recommendation as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps resource-recommendation <name> [flags]\n\nSuggests memory and CPU limits for an app based on its own historical\nusage (p95/p99 over a lookback window) and current limits, using a\ndeterministic engine, never an external model. Never changes anything;\nthe operator decides whether to act on the suggestion.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps resource-recommendation requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	rec, err := client.GetAppResourceRecommendation(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get resource recommendation for app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, rec); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printResourceRecommendationHuman(stdout, "app", rec)
	return exitOK
}
