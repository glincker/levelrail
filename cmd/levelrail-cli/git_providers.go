package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runGitProviders implements "git-providers": GET /api/v1/git-providers
// (internal/api/git_providers.go), a capability summary for every git
// provider (github, gitlab, bitbucket) in one call, replacing three
// separate "github-app status"/"gitlab-app status"/"bitbucket-app
// status" round-trips when a caller just wants an overview. Read-only,
// no subcommand, the same flat shape "containers" and "status" already
// use for a single GET with no verb of its own.
func runGitProviders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "git-providers", "print providers as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, gitProvidersUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	providers, err := client.ListGitProviders(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list git providers: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, providers, func() { printGitProvidersHuman(stdout, providers) })
}

func gitProvidersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s git-providers [flags]

Shows connection status and capabilities (list branches, register a
webhook, authenticated clone) for github, gitlab, and bitbucket in one
call. See "%[1]s github-app status", "%[1]s gitlab-app status", or
"%[1]s bitbucket-app status" for one provider's own fuller detail.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print providers as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func printGitProvidersHuman(out io.Writer, providers []gitProviderResource) {
	if len(providers) == 0 {
		_, _ = fmt.Fprintln(out, "no providers")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tCONNECTED\tCAN_LIST_BRANCHES\tCAN_REGISTER_WEBHOOK\tCAN_AUTH_CLONE")
	for _, p := range providers {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%v\t%v\t%v\n", p.Provider, p.Connected, p.CanListBranches, p.CanRegisterWebhook, p.CanAuthClone)
	}
	_ = tw.Flush()
}
