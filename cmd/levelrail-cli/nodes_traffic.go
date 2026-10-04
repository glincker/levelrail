package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesTraffic implements "nodes traffic": GET /api/v1/network/proxy,
// the Traffic dashboard page's own data source. Joins every app domain
// with where it actually runs, whether this control plane's own ingress
// can reach it (internal/reconcile/ingress's CrossNodeIngress condition),
// and its certificate status. Takes no positional argument.
func runNodesTraffic(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes traffic", "print the traffic/reachability report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesTrafficUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: nodes traffic takes no arguments\n\n", prog)
		_, _ = fmt.Fprint(stderr, nodesTrafficUsage(prog))
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	status, err := client.GetNetworkProxyStatus(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get network traffic status: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, status, func() { printNetworkProxyHuman(stdout, status) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNetworkProxyHuman(out io.Writer, s networkProxyResource) {
	if len(s.Domains) == 0 {
		_, _ = fmt.Fprintln(out, "no domains configured")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tAPP\tNODE\tREACHABLE\tTLS STATUS\tFIX")
	for _, d := range s.Domains {
		reachable := "yes"
		fix := "-"
		if !d.Reachable {
			reachable = "no"
			fix = d.FixCommand
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", d.Domain, d.App, orDash(d.NodeName), reachable, orDash(d.TLSStatus), fix)
	}
	_ = tw.Flush()
}

func nodesTrafficUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes traffic [flags]

Shows, per domain, which app it routes to, where that app actually
runs, whether this control plane's own ingress can reach it, and its
certificate status. A domain placed on a node this control plane's
ingress cannot yet reach shows reachable=no with a fix command.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the traffic/reachability report as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
