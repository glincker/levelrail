package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsList implements "apps list": GET /api/v1/apps, printed either
// as a compact table or as JSON. Kept deliberately small (the task this
// command exists for is verifying "apps create" worked, not being a
// full-featured resource browser), per this CLI's own scope: apps
// create is the point, list/get are minimal companions.
func runAppsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print apps as a JSON array to stdout and nothing else")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps list [flags]\n\nLists every app the caller's token can read.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	apps, err := client.ListApps(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, apps); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printAppsTable(stdout, apps)
	return exitOK
}
