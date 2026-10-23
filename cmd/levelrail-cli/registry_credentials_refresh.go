package main

import (
	"context"
	"fmt"
	"io"
)

// runRegistryCredentialsRefresh implements "registry-credentials refresh <id>":
// POST /api/v1/registry-credentials/{id}/refresh, minting a new login now.
func runRegistryCredentialsRefresh(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry-credentials refresh", "print the refreshed registry credential as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsRefreshUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "registry-credentials refresh", "registry credential id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.RefreshRegistryCredential(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("refresh registry credential %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		expiry := "no expiry reported"
		if updated.ExpiresAt != nil {
			expiry = "expires " + updated.ExpiresAt.UTC().Format("2006-01-02 15:04 MST")
		}
		_, _ = fmt.Fprintf(stdout, "registry credential %q refreshed, %s\n", updated.Name, expiry)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func registryCredentialsRefreshUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials refresh <id> [flags]

Mints a new login now for a cloud registry credential (ECR, Artifact
Registry or ACR), the same fix as the Refresh now button.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the refreshed registry credential as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
