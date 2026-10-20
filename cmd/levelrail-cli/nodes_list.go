package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runNodesList implements "nodes list": GET /api/v1/nodes, with
// optional q/limit/offset filtering, the same param names and shape
// "apps list"'s backend (internal/api/apps_list.go) already uses.
func runNodesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes list", "print nodes as a JSON array to stdout and nothing else", stderr)
	var opts apiclient.ListNodesOptions
	fs.StringVar(&opts.Query, "q", "", "search node name and address")
	fs.IntVar(&opts.Limit, "limit", 0, "max nodes to return (default: every node)")
	fs.IntVar(&opts.Offset, "offset", 0, "skip this many nodes before the page starts")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	nodes, _, err := client.ListNodesFiltered(context.Background(), opts)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list nodes: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, nodes, func() { printNodesTable(stdout, nodes) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes list [flags]

Lists nodes, optionally filtered and paged.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --q string                search node name and address
  --limit int               max nodes to return (default: every node)
  --offset int              skip this many nodes before the page starts
  --json                    print nodes as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
