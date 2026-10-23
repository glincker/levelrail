package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesSetLabels implements "nodes set-labels <id>": PUT
// /api/v1/nodes/{id}/region. Only flags that were passed are sent, so an
// omitted label keeps its value and an explicit empty one clears it.
func runNodesSetLabels(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes set-labels", "print the updated node as JSON to stdout and nothing else", stderr)
	var region, provider, zone string
	fs.StringVar(&region, "region", "", "region label (empty clears it)")
	fs.StringVar(&provider, "provider", "", "provider label (empty clears it)")
	fs.StringVar(&zone, "zone", "", "zone label (empty clears it)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesSetLabelsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes set-labels", "node id")
	if !ok {
		return exitUsage
	}

	var req setNodeFailureDomainReq
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "region":
			req.Region = &region
		case "provider":
			req.Provider = &provider
		case "zone":
			req.Zone = &zone
		}
	})
	if req.Region == nil && req.Provider == nil && req.Zone == nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("pass at least one of --region, --provider, --zone"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetNodeFailureDomain(context.Background(), id, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set labels for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stdout, "node %q labels set: provider=%q region=%q zone=%q\n", id, updated.Provider, updated.Region, updated.Zone)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesSetLabelsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes set-labels <id> [--region R] [--provider P] [--zone Z] [flags]

Sets a node's failure-domain labels. Placement prefers spreading replicas
across distinct zones, then regions. A flag you omit keeps its value; pass
an empty value (--zone=) to clear it.

Flags:
  --region string          region label
  --provider string        provider label
  --zone string            zone label
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the updated node as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
