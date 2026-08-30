package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runRegistryCredentialsCreate implements "registry-credentials create":
// POST /api/v1/registry-credentials. --password is always required here,
// unlike "registry-credentials update" where rotating it is optional.
func runRegistryCredentialsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "registry-credentials create", "print the created registry credential as JSON to stdout and nothing else", stderr)
	var name, registryHost, username, password string
	fs.StringVar(&name, "name", "", "display name for the registry credential (required)")
	fs.StringVar(&registryHost, "registry-host", "", "registry hostname, e.g. ghcr.io (required)")
	fs.StringVar(&username, "username", "", "registry username (required)")
	fs.StringVar(&password, "password", "", "registry password or token (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

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

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	created, err := client.CreateRegistryCredential(context.Background(), createRegistryCredentialRequest{
		Name: name, RegistryHost: registryHost, Username: username, Password: password,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create registry credential %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "registry credential %q (id %s, host %s) connected\n", created.Name, created.ID, created.RegistryHost)
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
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the created registry credential as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
