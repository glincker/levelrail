package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesDelete implements "nodes delete <id>": DELETE
// /api/v1/nodes/{id}. Refused with a 409 while any service or database
// is still placed on the node (handleDeleteNode's own doc comment):
// reportError below surfaces that response's real message as-is, which
// already tells the operator to drain first, rather than replacing it
// with a generic failure string.
func runNodesDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "nodes delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "nodes delete", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	if err := client.DeleteNode(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete node %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "node %q deleted\n", id)
	return exitOK
}

func nodesDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes delete <id> [flags]

Deletes a node. Refused (409) while any service or database is still
placed on it; drain it first with "%[1]s nodes drain <id>".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
