package main

import (
	"context"
	"fmt"
	"io"
)

// runRegistryCredentialsRepositories implements "registry-credentials
// repositories <id>": GET
// /api/v1/registry-credentials/{id}/repositories, browsing a stored
// external registry credential's own catalog the same way "registry
// repositories" (registry.go) browses the built-in one.
func runRegistryCredentialsRepositories(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry-credentials repositories", "print repositories as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsRepositoriesUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "registry-credentials repositories", "registry credential id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	repos, err := client.ListRegistryCredentialRepositories(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list registry credential %q repositories: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, repos, func() { printRegistryRepositories(stdout, repos) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func registryCredentialsRepositoriesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials repositories <id> [flags]

Lists every repository in the external registry the given credential
authenticates against.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print repositories as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
