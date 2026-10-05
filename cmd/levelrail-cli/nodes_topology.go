package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesTopology implements "nodes topology": GET
// /api/v1/network/topology, a read-only, whole-mesh summary (nodes,
// apps, databases, load balancers, app-to-database connections) backing
// the Network topology dashboard page. Takes no positional argument,
// the same no-argument shape "nodes mesh" uses: there is only ever one
// live topology to ask for.
func runNodesTopology(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes topology", "print the topology summary as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesTopologyUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: nodes topology takes no arguments\n\n", prog)
		_, _ = fmt.Fprint(stderr, nodesTopologyUsage(prog))
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	topo, err := client.GetNetworkTopology(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get network topology: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, topo, func() { printNetworkTopologyHuman(stdout, topo) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNetworkTopologyHuman(out io.Writer, t networkTopologyResource) {
	_, _ = fmt.Fprintf(out, "zone: %s   mesh enabled: %t\n\n", orDash(t.Zone), t.MeshEnabled)

	_, _ = fmt.Fprintln(out, "NODES")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tREGION\tSTATUS\tSCHEDULABLE\tLOCAL\tMESH ADDRESS")
	for _, n := range t.Nodes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%t\t%s\n", n.ID, n.Name, orDash(n.Region), n.Status, n.Schedulable, n.IsLocal, orDash(n.MeshAddress))
	}
	_ = tw.Flush()

	_, _ = fmt.Fprintln(out, "\nAPPS")
	tw = tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tNODE\tDNS NAME\tMESH ADDRESS\tDOMAINS")
	for _, a := range t.Apps {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.Name, orDash(a.NodeID), orDash(a.DNSName), orDash(a.MeshAddress), joinOrDash(a.Domains))
	}
	_ = tw.Flush()

	if len(t.Databases) > 0 {
		_, _ = fmt.Fprintln(out, "\nDATABASES")
		tw = tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tENGINE\tNODE\tDNS NAME\tMESH ADDRESS")
		for _, d := range t.Databases {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.Name, d.Engine, orDash(d.NodeID), orDash(d.DNSName), orDash(d.MeshAddress))
		}
		_ = tw.Flush()
	}

	if len(t.Connections) > 0 {
		_, _ = fmt.Fprintln(out, "\nCONNECTIONS")
		tw = tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "APP\tDATABASE\tENV VAR")
		for _, c := range t.Connections {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.App, c.Database, orDash(c.EnvVar))
		}
		_ = tw.Flush()
	}
}

func joinOrDash(ss []string) string {
	if len(ss) == 0 {
		return "-"
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += ", " + s
	}
	return out
}

func nodesTopologyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes topology [flags]

Shows a read-only, whole-mesh summary: every node, app, database, load
balancer, and app-to-database connection, with internal DNS names and
mesh addresses. Every fact here already exists via GET /apps,
/databases, /nodes, /loadbalancers; this just joins them into one
graph-shaped view.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the topology summary as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
