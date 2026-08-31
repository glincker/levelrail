package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runRegistryCredentialsTest implements "registry-credentials test <id>":
// POST /api/v1/registry-credentials/{id}/test, authenticating the stored
// credential against its registry host without pulling anything.
func runRegistryCredentialsTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "registry-credentials test", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsTestUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "registry-credentials test", "registry credential id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	if err := client.TestRegistryCredential(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test registry credential %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"ok": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "registry credential %q authenticated successfully\n", id)
	return exitOK
}

func registryCredentialsTestUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials test <id> [flags]

Authenticates a connected registry credential against its registry host,
without pulling an image, catching a bad or stale credential before a
deploy fails mid-pull.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {"ok": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
