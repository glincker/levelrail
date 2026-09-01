package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsNetwork implements "apps network <name>": GET
// /api/v1/apps/{name}/network (internal/api/network.go's
// handleGetAppNetwork), the live traffic path, container's declared port
// plus whatever host port Docker currently has bound. A separate command
// from "apps get" for the same reason "apps status" is: this is observed
// state, not the desired appResource contract "apps get --json" already
// promises callers.
func runAppsNetwork(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps network", "print the network status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps network <name> [flags]\n\nShows how traffic actually reaches this app right now: container port,\nlive Docker-assigned host port, and whether it's running.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps network", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	network, err := client.GetAppNetwork(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get network for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, network, func() { printAppNetworkHuman(stdout, network) })
}
