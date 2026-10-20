package main

import (
	"context"
	"fmt"
	"io"
)

// runRegistryCredentialsGet implements "registry-credentials get <id>":
// GET /api/v1/registry-credentials/{id}.
func runRegistryCredentialsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry-credentials get", "print the registry credential as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsGetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "registry-credentials get", "registry credential id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	cred, err := client.GetRegistryCredential(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get registry credential %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, cred, func() { printRegistryCredentialHuman(stdout, cred) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the registry credential as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
