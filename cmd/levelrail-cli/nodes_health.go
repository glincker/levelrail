package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// nodesHealthResponse is "nodes health"'s combined --json shape: the
// stored reconcile conditions plus a live re-check of whether any
// node-scoped alert kind is currently firing for this specific node, so
// one command answers "is this node unhealthy right now" without a
// second, separate query against an app-scoped alerts list.
type nodesHealthResponse struct {
	Conditions  []conditionResource      `json:"conditions"`
	AlertStatus *nodeAlertStatusResource `json:"alert_status,omitempty"`
}

// runNodesHealth implements "nodes health <id>": GET
// /api/v1/nodes/{id}/health for the node health controller's stored
// reconcile conditions (printed with the same printConditionsHuman table
// "apps status" already uses), plus GET /api/v1/nodes/{id}'s live
// alert_status, so this one command covers both "is the node reachable"
// and "has a platform-wide alert rule actually fired because of this
// node" in one place.
func runNodesHealth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "nodes health", "print conditions and alert status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesHealthUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "nodes health", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	conditions, err := client.GetNodeHealth(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get health for node %q: %w", id, err))
	}

	// A node's alert_status is best-effort here: if the node lookup
	// fails, the conditions above are still worth printing, so this
	// falls back to nil (omitted) rather than failing the whole command.
	var alertStatus *nodeAlertStatusResource
	if node, err := client.GetNode(context.Background(), id); err == nil {
		alertStatus = node.AlertStatus
	}

	if jsonOut {
		if err := writeJSONValue(stdout, nodesHealthResponse{Conditions: conditions, AlertStatus: alertStatus}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printConditionsHuman(stdout, conditions)
	if alertStatus != nil {
		_, _ = fmt.Fprintf(stdout, "\nalert status:\n")
		_, _ = fmt.Fprintf(stdout, "  patch status:          %s\n", alertStatus.PatchStatus)
		_, _ = fmt.Fprintf(stdout, "  node disk space:       %s\n", alertStatus.NodeDiskSpace)
		_, _ = fmt.Fprintf(stdout, "  node resource usage:   %s\n", alertStatus.NodeResourceUsage)
	}
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
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print conditions as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
