package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesGet implements "nodes get <id>": GET /api/v1/nodes/{id}.
func runNodesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "nodes get", "print the node as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesGetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "nodes get", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	node, err := client.GetNode(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get node %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, node); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printNodeHuman(stdout, node)
	return exitOK
}

func nodesGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes get <id> [flags]

Shows one node's current state.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the node as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
