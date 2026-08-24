package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesList implements "nodes list": GET /api/v1/nodes.
func runNodesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "nodes list", "print nodes as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	nodes, err := client.ListNodes(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list nodes: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, nodes); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printNodesTable(stdout, nodes)
	return exitOK
}

func nodesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes list [flags]

Lists every node.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print nodes as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
