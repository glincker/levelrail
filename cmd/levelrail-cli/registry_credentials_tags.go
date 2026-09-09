package main

import (
	"context"
	"fmt"
	"io"
)

// runRegistryCredentialsTags implements "registry-credentials tags <id>
// <repository>": GET
// /api/v1/registry-credentials/{id}/tags?repository=<name>, the same
// browsing shape runRegistryCredentialsRepositories establishes for one
// specific repository.
func runRegistryCredentialsTags(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry-credentials tags", "print tags as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsTagsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "registry-credentials tags", "a registry credential id and a repository name", 2)
	if !ok {
		return exitUsage
	}
	id, repository := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	tags, err := client.ListRegistryCredentialTags(context.Background(), id, repository)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list registry credential %q tags for %q: %w", id, repository, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, tags, func() { printRegistryTags(stdout, tags) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func registryCredentialsTagsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials tags <id> <repository> [flags]

Lists every tag pushed for one repository in the external registry the
given credential authenticates against.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print tags as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
