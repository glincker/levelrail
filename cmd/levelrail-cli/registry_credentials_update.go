package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runRegistryCredentialsUpdate implements "registry-credentials update
// <id>": PUT /api/v1/registry-credentials/{id}, a full replace of name/
// registry_host/username. --password is optional here, unlike
// "registry-credentials create": omitted, the credential keeps its
// existing stored password; given, it rotates it.
func runRegistryCredentialsUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "registry-credentials update", "print the updated registry credential as JSON to stdout and nothing else", stderr)
	var name, registryHost, username, password string
	fs.StringVar(&name, "name", "", "display name for the registry credential (required)")
	fs.StringVar(&registryHost, "registry-host", "", "registry hostname, e.g. ghcr.io (required)")
	fs.StringVar(&username, "username", "", "registry username (required)")
	fs.StringVar(&password, "password", "", "new registry password or token; omit to keep the existing one")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsUpdateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

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

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	updated, err := client.UpdateRegistryCredential(context.Background(), id, updateRegistryCredentialRequest{
		Name: name, RegistryHost: registryHost, Username: username, Password: password,
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

Updates a registry credential's name/registry host/username. Add
--password to rotate its stored password in the same call; omit it to
leave the password unchanged.

Flags:
  --name string             display name for the registry credential (required)
  --registry-host string    registry hostname, e.g. ghcr.io (required)
  --username string         registry username (required)
  --password string         new registry password or token, omit to keep the existing one
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the updated registry credential as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
