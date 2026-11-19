package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// The plaintext token is returned once and never recoverable, so it is printed with a warning.
func runNodesJoinToken(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes join-token", "print the new join token as JSON to stdout and nothing else", stderr)
	var req apiclient.CreateNodeJoinTokenRequest
	fs.BoolVar(&req.Builder, "builder", false, "enrol a build-only personal builder that never receives app workloads")
	fs.StringVar(&req.BuildTrust, "build-trust", "", "builder tier: server (default), managed or personal")
	fs.StringVar(&req.NodeName, "name", "", "name the new node enrolls with")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesJoinTokenUsage(prog)) }
	if len(args) > 0 && args[0] == "create" {
		args = args[1:]
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.CreateNodeJoinTokenFor(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create node join token: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "join token value (shown once, not recoverable again): %s\n", created.Token)
		_, _ = fmt.Fprintf(stdout, "expires at: %s\n", created.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
		if created.CAFingerprint != "" {
			_, _ = fmt.Fprintf(stdout, "control plane CA fingerprint (set APP_CA_FINGERPRINT on the agent): %s\n", created.CAFingerprint)
		}
		if created.JoinCommands != nil {
			_, _ = fmt.Fprintln(stdout)
			printJoinCommands(stdout, created, installAgentOpts{Name: req.NodeName})
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesJoinTokenUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes join-token [create] [--builder] [flags]

Mints a one-time enrollment token for a new node, printed once (never
recoverable from the server again after this call).

Flags:
  --builder                enrol a build-only personal builder (never gets app workloads)
  --build-trust string     builder tier: server (default), managed or personal
  --name string            name the new node enrolls with
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the new join token as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
