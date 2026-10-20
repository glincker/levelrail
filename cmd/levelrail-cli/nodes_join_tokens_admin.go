package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runNodesJoinTokens implements "nodes join-tokens": GET /api/v1/nodes/join-tokens.
func runNodesJoinTokens(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes join-tokens", "print the pending tokens as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesJoinTokensUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	toks, err := client.ListPendingJoinTokens(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list pending join tokens: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, toks, func() {
		if len(toks) == 0 {
			_, _ = fmt.Fprintln(stdout, "no pending join tokens")
			return
		}
		for _, t := range toks {
			_, _ = fmt.Fprintf(stdout, "%s  %s  expires %s\n", t.ID, t.Purpose, t.ExpiresAt.Format(time.RFC3339))
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesRevokeJoinToken implements "nodes revoke-join-token <id> | --all".
func runNodesRevokeJoinToken(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes revoke-join-token", "print the result as JSON to stdout and nothing else", stderr)
	all := fs.Bool("all", false, "revoke every pending join token")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesRevokeJoinTokenUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id := ""
	switch rest := fs.Args(); {
	case *all && len(rest) == 0:
	case !*all && len(rest) == 1:
		id = rest[0]
	default:
		_, _ = fmt.Fprint(stderr, nodesRevokeJoinTokenUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.RevokeJoinTokens(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke join tokens: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "revoked %d pending join token(s)\n", res.Revoked)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesJoinTokensUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes join-tokens [flags]

Lists join tokens that are unused and not yet expired. The secret values are
never shown.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the pending tokens as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func nodesRevokeJoinTokenUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes revoke-join-token <id> [flags]
  %[1]s nodes revoke-join-token --all [flags]

Revokes pending join tokens so a leaked or abandoned enrollment command stops
working at once. Nodes that already enrolled are not affected.

Flags:
  --all                     revoke every pending join token
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
