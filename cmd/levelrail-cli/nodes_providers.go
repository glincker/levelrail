package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runNodesProviders dispatches "nodes providers <verb> [flags]" to one
// of list/set-credential: internal/api/node_provision.go's cloud
// provider credential routes, the two-level shape "nodes <resource>
// <verb>" iam.go's own doc comment already establishes for "iam
// policies".
func runNodesProviders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nodesProvidersUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nodesProvidersUsage(prog))
		return exitOK
	case "list":
		return runNodesProvidersList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set-credential":
		return runNodesProvidersSetCredential(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown nodes providers subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, nodesProvidersUsage(prog))
		return exitUsage
	}
}

func nodesProvidersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes providers list [flags]                                          list known cloud providers and whether each has a stored credential
  %[1]s nodes providers set-credential --provider NAME --provider-token TOKEN [flags]   store (or replace) a provider's API token

NAME must be "hetzner" or "digitalocean".

Run "%[1]s nodes providers <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runNodesProvidersList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes providers list", "print providers as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesProvidersListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	providers, err := client.ListNodeProviders(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list node providers: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, providers, func() { printNodeProvidersTable(stdout, providers) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesProvidersListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes providers list [flags]

Lists every known cloud provider (hetzner, digitalocean) and whether a
credential is currently stored for it.

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

func printNodeProvidersTable(out io.Writer, providers []nodeProviderResource) {
	if len(providers) == 0 {
		_, _ = fmt.Fprintln(out, "no providers")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tHAS TOKEN")
	for _, p := range providers {
		_, _ = fmt.Fprintf(tw, "%s\t%t\n", p.Provider, p.HasToken)
	}
	_ = tw.Flush()
}

func runNodesProvidersSetCredential(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes providers set-credential", "print the updated provider as JSON to stdout and nothing else", stderr)
	var provider, providerToken string
	fs.StringVar(&provider, "provider", "", "hetzner or digitalocean (required)")
	fs.StringVar(&providerToken, "provider-token", "", "the provider's API token (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesProvidersSetCredentialUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	if providerToken == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider-token is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	updated, err := client.SetNodeProviderCredential(context.Background(), setNodeProviderCredentialRequest{Provider: provider, Token: providerToken})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set credential for provider %q: %w", provider, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stdout, "provider %q credential stored\n", updated.Provider)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesProvidersSetCredentialUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes providers set-credential --provider NAME --provider-token TOKEN [flags]

Stores (or replaces) a cloud provider's API token, encrypted the same way
every other integration credential in this platform is. The token is
never echoed back.

Flags:
  --provider string       hetzner or digitalocean (required)
  --provider-token string  the provider's API token (required)
  --api-url string       control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated provider as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}
