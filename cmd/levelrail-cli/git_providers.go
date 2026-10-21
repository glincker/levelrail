package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Read-only flat GET: one call replaces the four per-provider status commands.
func runGitProviders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && args[0] == "validate" {
		return runGitProvidersValidate(prog, args[1:], stdout, stderr, lookupEnv)
	}
	if len(args) > 0 && args[0] == "repos" {
		return runGitProvidersRepos(prog, args[1:], stdout, stderr, lookupEnv)
	}
	if len(args) > 0 {
		switch args[0] {
		case "connections", "add", "test", "rename", "default", "disconnect", "branches", "use-as-source":
			return runGitConnectionsCmd(prog, args[0], args[1:], stdout, stderr, lookupEnv)
		}
	}
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
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

	return writeScheduledTaskResult(stdout, stderr, of, providers, func() {
		printGitProvidersHuman(stdout, providers)
		conns, err := client.ListGitConnections(context.Background(), "")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: could not list connections: %v\n", err)
			return
		}
		_, _ = fmt.Fprintln(stdout)
		printGitConnectionsHuman(stdout, conns)
	})
}

func gitProvidersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s git-providers [list] [flags]
  %[1]s git-providers validate <github|gitlab|gitea|bitbucket> [flags]
  %[1]s git-providers repos <github|gitlab|gitea|bitbucket> [--connection ID] [--search Q] [--refresh] [flags]
  %[1]s git-providers connections|add|rename|default|disconnect|branches|use-as-source ...

Shows connection status and capabilities (list branches, register a
webhook, authenticated clone) for github, gitlab, bitbucket, and gitea,
then every connected account. Run "%[1]s git-providers connections -h"
for managing several accounts per provider.

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
	_, _ = fmt.Fprintln(tw, "PROVIDER\tCONNECTED\tHEALTH\tCAN_LIST_BRANCHES\tCAN_REGISTER_WEBHOOK\tCAN_AUTH_CLONE")
	for _, p := range providers {
		health := p.Connection.Status
		if health == "" {
			health = "ok"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\t%v\t%v\t%v\n", p.Provider, p.Connected, health, p.CanListBranches, p.CanRegisterWebhook, p.CanAuthClone)
	}
	_ = tw.Flush()
	for _, p := range providers {
		printGitReconnectHint(out, p)
	}
}

// printGitReconnectHint says how to recover a connection that needs the
// operator. Connecting is a browser redirect, so the dashboard button is the
// way; the CLI line only names the page to open.
func printGitReconnectHint(out io.Writer, p gitProviderResource) {
	switch p.Connection.Status {
	case "needs_reauthorize", "revoked":
		_, _ = fmt.Fprintf(out, "%s: %s. Press Reconnect on its settings page in the dashboard, or open /api/v1/%s-app/connect on your control plane in a browser.\n", p.Provider, gitConnectionSummary(p.Connection), p.Provider)
	case "rate_limited":
		next := ""
		if p.Connection.NextCheckAt != nil {
			next = " until " + p.Connection.NextCheckAt.Format("15:04:05")
		}
		_, _ = fmt.Fprintf(out, "%s: the provider is rate limiting; saved lists are used%s.\n", p.Provider, next)
	}
}

func gitConnectionSummary(c apiclient.GitConnectionResource) string {
	if c.Detail != "" {
		return c.Status + " (" + c.Detail + ")"
	}
	return c.Status
}
