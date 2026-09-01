package main

import (
	"context"
	"fmt"
	"io"
)

// runRegistryCredentialsTest implements "registry-credentials test <id>":
// POST /api/v1/registry-credentials/{id}/test, authenticating the stored
// credential against its registry host without pulling anything.
func runRegistryCredentialsTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry-credentials test", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsTestUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "registry-credentials test", "registry credential id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.TestRegistryCredential(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test registry credential %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"ok": true}, func() {
		_, _ = fmt.Fprintf(stdout, "registry credential %q authenticated successfully\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"ok": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
