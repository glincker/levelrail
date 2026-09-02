package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// runContainers implements "containers": GET /api/v1/system/containers,
// every container on this node whether or not Levelrail manages it.
// Read-only: see internal/api/containers.go's own doc comment for why
// this deliberately has no stop/restart action of its own, use
// "apps stop"/"apps start"/"apps restart" for a Levelrail-managed app
// instead, which update desired state correctly.
func runContainers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "containers", "print containers as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, containersUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	containers, err := client.ListContainers(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list containers: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, containers, func() { printContainersHuman(stdout, containers) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printContainersHuman(out io.Writer, containers []containerResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATUS\tNAME\tIMAGE\tPORTS")
	for _, c := range containers {
		status := "stopped"
		if c.Running {
			status = "running"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", status, c.Name, c.Image, formatContainerPorts(c.Ports))
	}
	_ = tw.Flush()
}

func formatContainerPorts(ports []containerPortResource) string {
	if len(ports) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d->%d/%s", p.HostPort, p.ContainerPort, p.Protocol))
	}
	return strings.Join(parts, ", ")
}

func containersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s containers [flags]

Lists every container on this node, whether or not it's managed by
%[1]s. Read-only: to stop, start, or restart a %[1]s-managed app, use
"%[1]s apps stop/start/restart <name>" instead, which updates desired
state correctly rather than fighting the reconciler.

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print containers as a JSON array to stdout, nothing else
  --output string        output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
