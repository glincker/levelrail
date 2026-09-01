package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runRegistryCredentialsCreate implements "registry-credentials create":
// POST /api/v1/registry-credentials. --password is always required here,
// unlike "registry-credentials update" where rotating it is optional.
func runRegistryCredentialsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry-credentials create", "print the created registry credential as JSON to stdout and nothing else", stderr)
	var name, registryHost, username, password, expiresAt string
	fs.StringVar(&name, "name", "", "display name for the registry credential (required)")
	fs.StringVar(&registryHost, "registry-host", "", "registry hostname, e.g. ghcr.io (required)")
	fs.StringVar(&username, "username", "", "registry username (required)")
	fs.StringVar(&password, "password", "", "registry password or token (required)")
	fs.StringVar(&expiresAt, "expires-at", "", "optional RFC3339 expiry the operator already knows, e.g. from a GitHub PAT")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if registryHost == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--registry-host is required"))
	}
	if username == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--username is required"))
	}
	if password == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--password is required"))
	}
	var expiresAtPtr *time.Time
	if expiresAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, expiresAt)
		if parseErr != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("--expires-at must be RFC3339: %v", parseErr))
		}
		expiresAtPtr = &parsed
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.CreateRegistryCredential(context.Background(), createRegistryCredentialRequest{
		Name: name, RegistryHost: registryHost, Username: username, Password: password, ExpiresAt: expiresAtPtr,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create registry credential %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "registry credential %q (id %s, host %s) connected\n", created.Name, created.ID, created.RegistryHost)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func registryCredentialsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials create --name NAME --registry-host HOST --username USER --password PASS [flags]

Connects a new registry credential.

Flags:
  --name string             display name for the registry credential (required)
  --registry-host string    registry hostname, e.g. ghcr.io (required)
  --username string         registry username (required)
  --password string         registry password or token (required)
  --expires-at string       optional RFC3339 expiry the operator already knows
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the created registry credential as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
