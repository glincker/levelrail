package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Lists one row per replica container with live state, ports and usage.
func runAppsContainers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps containers", "print the containers as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps containers <name> [flags]\n\nLists every replica container of an app: state, image, ports, networks,\nmounts, limits and live usage. Fields a node cannot report are named\nunder \"unavailable\" in --json.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps containers", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	containers, err := client.ListAppContainers(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list containers for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, containers, func() { printAppContainersHuman(stdout, containers) })
}

func printAppContainersHuman(out io.Writer, containers []apiclient.AppContainer) {
	if len(containers) == 0 {
		_, _ = fmt.Fprintln(out, "no containers")
		return
	}
	_, _ = fmt.Fprintf(out, "%-12s  %-8s  %-28s  %-10s  %s\n", "ID", "STATE", "IMAGE", "NODE", "PORTS")
	for _, c := range containers {
		ports := make([]string, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.Bind, p.Host, p.Container, p.Proto))
		}
		_, _ = fmt.Fprintf(out, "%-12s  %-8s  %-28s  %-10s  %s\n", c.ID, c.State, c.Image, c.Node, strings.Join(ports, ","))
	}
}
