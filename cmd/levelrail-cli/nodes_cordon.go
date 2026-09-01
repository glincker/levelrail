package main

import (
	"context"
	"fmt"
	"io"
)

// runNodesCordon implements "nodes cordon <id>": POST
// /api/v1/nodes/{id}/cordon.
func runNodesCordon(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runNodesSetSchedulable(prog, "cordon", args, stdout, stderr, lookupEnv)
}

// runNodesUncordon implements "nodes uncordon <id>": POST
// /api/v1/nodes/{id}/uncordon, the inverse of runNodesCordon, mirroring
// how handleCordonNode/handleUncordonNode share one file server-side.
func runNodesUncordon(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runNodesSetSchedulable(prog, "uncordon", args, stdout, stderr, lookupEnv)
}

// runNodesSetSchedulable is the shared implementation behind both
// runNodesCordon and runNodesUncordon: same flags, same flow, only the
// verb and the client method called differ.
func runNodesSetSchedulable(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "nodes "+verb, "print the updated node as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesCordonUsage(prog, verb)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "nodes "+verb, "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	var node nodeResource
	var err error
	if verb == "cordon" {
		node, err = client.CordonNode(context.Background(), id)
	} else {
		node, err = client.UncordonNode(context.Background(), id)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s node %q: %w", verb, id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, node); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "node %q schedulable: %t\n", id, node.Schedulable)
	return exitOK
}

func nodesCordonUsage(prog, verb string) string {
	desc := "Marks a node unschedulable for new placements, without evacuating anything already running there."
	if verb == "uncordon" {
		desc = "Marks a node schedulable again, allowing new placements."
	}
	return fmt.Sprintf(`Usage:
  %[1]s nodes %[5]s <id> [flags]

%[6]s

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated node as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, verb, desc)
}
