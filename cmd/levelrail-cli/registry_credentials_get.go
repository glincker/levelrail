package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runRegistryCredentialsGet implements "registry-credentials get <id>":
// GET /api/v1/registry-credentials/{id}.
func runRegistryCredentialsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "registry-credentials get", "print the registry credential as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsGetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "registry-credentials get", "registry credential id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	cred, err := client.GetRegistryCredential(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get registry credential %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, cred); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printRegistryCredentialHuman(stdout, cred)
	return exitOK
}

func printRegistryCredentialHuman(out io.Writer, c registryCredentialResource) {
	_, _ = fmt.Fprintf(out, "id:             %s\n", c.ID)
	_, _ = fmt.Fprintf(out, "name:           %s\n", c.Name)
	_, _ = fmt.Fprintf(out, "registry host:  %s\n", c.RegistryHost)
	_, _ = fmt.Fprintf(out, "username:       %s\n", c.Username)
	_, _ = fmt.Fprintf(out, "created:        %s\n", c.CreatedAt)
}

func registryCredentialsGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials get <id> [flags]

Shows one registry credential. The password is never included.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the registry credential as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
