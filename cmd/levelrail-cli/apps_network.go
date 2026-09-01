package main

import (
	"context"
	"flag"
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
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps network", "print the network status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps network <name> [flags]\n\nShows how traffic actually reaches this app right now: container port,\nlive Docker-assigned host port, and whether it's running.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps network requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	network, err := client.GetAppNetwork(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get network for app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, network); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printAppNetworkHuman(stdout, network)
	return exitOK
}
