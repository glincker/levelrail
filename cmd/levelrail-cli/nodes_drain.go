package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesDrain implements "nodes drain <id> [--target ID]": POST
// /api/v1/nodes/{id}/drain?target_node_id=. --target omitted matches
// the API's own default (the empty string, the local-node sentinel). A
// partial failure is not a network or API-level error (handleDrainNode
// returns 200 or 207, never a failure status for this): the same
// "print the result, then classify AllSucceeded" exit-code convention
// "apps deploy-spec" already uses for its own per-item result.
func runNodesDrain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "nodes drain", "print the drain result as JSON to stdout and nothing else", stderr)
	var target string
	fs.StringVar(&target, "target", "", "node id to move placements to (default: the local node)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesDrainUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "nodes drain", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.DrainNode(context.Background(), id, target)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("drain node %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, result); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		if len(result.Errors) > 0 {
			return exitAPIError
		}
		return exitOK
	}
	printDrainNodeResultHuman(stdout, id, result)
	if len(result.Errors) > 0 {
		return exitAPIError
	}
	return exitOK
}

func printDrainNodeResultHuman(out io.Writer, id string, r drainNodeResponse) {
	target := r.TargetNodeID
	if target == "" {
		target = "(local)"
	}
	_, _ = fmt.Fprintf(out, "node %q drained to %s\n", id, target)

	if len(r.MovedServices) == 0 && len(r.MovedDatabases) == 0 {
		_, _ = fmt.Fprintln(out, "nothing was placed on this node")
	} else {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tNAME")
		for _, s := range r.MovedServices {
			_, _ = fmt.Fprintf(tw, "service\t%s\n", s)
		}
		for _, d := range r.MovedDatabases {
			_, _ = fmt.Fprintf(tw, "database\t%s\n", d)
		}
		_ = tw.Flush()
	}

	if len(r.Errors) > 0 {
		_, _ = fmt.Fprintln(out, "errors:")
		for _, e := range r.Errors {
			_, _ = fmt.Fprintf(out, "  %s\n", e)
		}
	}
}

func nodesDrainUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes drain <id> [--target NODE-ID] [flags]

Moves every service and database currently placed on <id> to
--target (default: the local node). Only changes desired placement;
the reconcile engine's next pass converges each moved resource on its
new node. A resource that fails to move is reported, not silently
dropped; everything that did move stays moved.

Flags:
  --target string          node id to move placements to (default: the local node)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the drain result as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
