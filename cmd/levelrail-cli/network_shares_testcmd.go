package main

import (
	"context"
	"fmt"
	"io"
)

// runNetworkSharesTest implements "network-shares test <id>": POST
// /api/v1/network-shares/{id}/test, dialing the share's host on its
// protocol's standard port. Reachability only, never an authentication
// check: a closed port makes any later mount fail regardless of
// whether a cifs credential is correct.
func runNetworkSharesTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "network-shares test", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, networkSharesTestUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "network-shares test", "network share id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.TestNetworkShare(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test network share %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"ok": true}, func() {
		_, _ = fmt.Fprintf(stdout, "network share %q is reachable\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func networkSharesTestUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s network-shares test <id> [flags]

Dials a connected network share's host on its protocol's standard port
(NFS 2049, CIFS 445) to confirm it is reachable, before anyone attaches
it to an app's volume. Does not check credentials.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"ok": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
