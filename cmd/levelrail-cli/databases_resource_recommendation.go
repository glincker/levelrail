package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesResourceRecommendation implements "databases
// resource-recommendation <name>": GET
// /api/v1/databases/{name}/resource-recommendation
// (internal/api/database_resource_recommendation.go's own
// handleDatabaseResourceRecommendation), the database-kind counterpart to
// runAppsResourceRecommendation (apps_resource_recommendation.go).
func runDatabasesResourceRecommendation(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "databases resource-recommendation", "print the recommendation as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases resource-recommendation <name> [flags]\n\nSuggests memory and CPU limits for a database based on its own\nhistorical usage (p95/p99 over a lookback window) and current limits,\nusing a deterministic engine, never an external model. Never changes\nanything; the operator decides whether to act on the suggestion.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP}, stderr, singleArgCmd{prog, "databases resource-recommendation", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	rec, err := client.GetDatabaseResourceRecommendation(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get resource recommendation for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, rec, func() { printResourceRecommendationHuman(stdout, "database", rec) })
}
