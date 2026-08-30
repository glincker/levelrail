package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesJoinToken implements "nodes join-token": POST
// /api/v1/nodes/join-tokens. handleCreateNodeJoinToken's response
// returns the plaintext token exactly once (its own doc comment: "the
// only way to redeem a token", never recoverable from the server
// again), the same one-time-secret contract "tokens create" already
// prints with an explicit note that it will not be shown again.
func runNodesJoinToken(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "nodes join-token", "print the new join token as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesJoinTokenUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	created, err := client.CreateNodeJoinToken(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create node join token: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "join token value (shown once, not recoverable again): %s\n", created.Token)
	_, _ = fmt.Fprintf(stdout, "expires at: %s\n", created.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
	return exitOK
}

func nodesJoinTokenUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes join-token [flags]

Mints a one-time enrollment token for a new node, printed once (never
recoverable from the server again after this call).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the new join token as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
