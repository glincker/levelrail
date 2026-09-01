package main

import (
	"context"
	"flag"
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
	fs := flag.NewFlagSet(prog+" databases resource-recommendation", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	var profileFlag string
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print the recommendation as JSON to stdout and nothing else")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases resource-recommendation <name> [flags]\n\nSuggests memory and CPU limits for a database based on its own\nhistorical usage (p95/p99 over a lookback window) and current limits,\nusing a deterministic engine, never an external model. Never changes\nanything; the operator decides whether to act on the suggestion.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: databases resource-recommendation requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))

	rec, err := client.GetDatabaseResourceRecommendation(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get resource recommendation for database %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, rec); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printResourceRecommendationHuman(stdout, "database", rec)
	return exitOK
}
