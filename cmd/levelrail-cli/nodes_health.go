package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesHealth implements "nodes health <id>": GET
// /api/v1/nodes/{id}/health, the node health controller's stored
// reconcile conditions, printed with the same printConditionsHuman
// table "apps status" already uses for an app's own conditions.
func runNodesHealth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "nodes health", "print conditions as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesHealthUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "nodes health", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	conditions, err := client.GetNodeHealth(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get health for node %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, conditions); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printConditionsHuman(stdout, conditions)
	return exitOK
}

func nodesHealthUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes health <id> [flags]

Shows a node's current stored reconcile conditions (current status,
not a history log).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print conditions as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
