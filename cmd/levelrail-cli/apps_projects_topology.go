package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsProjectsTopology implements "apps projects topology <id>": GET
// /api/v1/projects/{id}/topology, a diagram-ready graph of one
// project's apps, databases, and shared volumes, derived from already
// stored desired state. Every edge traces to one real stored field
// (database binding, depends_on, volume attachment, egress allow); see
// docs/service-topology-graph.md for what each edge kind means.
func runAppsProjectsTopology(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps projects topology", "print the topology graph as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsTopologyUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "apps projects topology", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	graph, err := client.GetProjectTopology(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get project %q topology: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, graph, func() { printProjectTopologyHuman(stdout, graph) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printProjectTopologyHuman(out io.Writer, g projectTopologyResource) {
	if len(g.Nodes) == 0 {
		_, _ = fmt.Fprintln(out, "no apps, databases, or volumes in this project")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tLABEL\tSTATUS")
	for _, n := range g.Nodes {
		status := "-"
		if n.Status != nil {
			status = n.Status.Label
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", n.ID, n.Kind, n.Label, status)
	}
	_ = tw.Flush()

	if len(g.Edges) > 0 {
		_, _ = fmt.Fprintln(out, "\nEDGES")
		tw = tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "FROM\tTO\tKIND")
		for _, e := range g.Edges {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", e.From, e.To, e.Kind)
		}
		_ = tw.Flush()
	}
}

func appsProjectsTopologyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects topology <id> [flags]

Shows a diagram-ready graph of one project's apps, databases, and
shared volumes: each node is an app/database/volume, each edge a real
relationship (database binding, depends_on, volume attachment, egress
allow), never invented.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the topology graph as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
