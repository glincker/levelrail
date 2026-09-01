package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runRegistryCredentialsUpdate implements "registry-credentials update
// <id>": PUT /api/v1/registry-credentials/{id}, a full replace of name/
// registry_host/username. --password is optional here, unlike
// "registry-credentials create": omitted, the credential keeps its
// existing stored password; given, it rotates it.
func runRegistryCredentialsUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "registry-credentials update", "print the updated registry credential as JSON to stdout and nothing else", stderr)
	var name, registryHost, username, password, expiresAt string
	fs.StringVar(&name, "name", "", "display name for the registry credential (required)")
	fs.StringVar(&registryHost, "registry-host", "", "registry hostname, e.g. ghcr.io (required)")
	fs.StringVar(&username, "username", "", "registry username (required)")
	fs.StringVar(&password, "password", "", "new registry password or token; omit to keep the existing one")
	fs.StringVar(&expiresAt, "expires-at", "", "optional RFC3339 expiry the operator already knows; omit to clear it")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsUpdateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "registry-credentials update", "registry credential id")
	if !ok {
		return exitUsage
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
	var expiresAtPtr *time.Time
	if expiresAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, expiresAt)
		if parseErr != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("--expires-at must be RFC3339: %v", parseErr))
		}
		expiresAtPtr = &parsed
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	updated, err := client.UpdateRegistryCredential(context.Background(), id, updateRegistryCredentialRequest{
		Name: name, RegistryHost: registryHost, Username: username, Password: password, ExpiresAt: expiresAtPtr,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update registry credential %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "registry credential %q (id %s) updated\n", updated.Name, updated.ID)
	return exitOK
}

func registryCredentialsUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials update <id> --name NAME --registry-host HOST --username USER [flags]

Updates a registry credential's name/registry host/username/expiry. Add
--password to rotate its stored password in the same call; omit it to
leave the password unchanged. --expires-at is a full replace like the
other fields: omit it to clear a previously-set expiry.

Flags:
  --name string             display name for the registry credential (required)
  --registry-host string    registry hostname, e.g. ghcr.io (required)
  --username string         registry username (required)
  --password string         new registry password or token, omit to keep the existing one
  --expires-at string       optional RFC3339 expiry, omit to clear it
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the updated registry credential as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
