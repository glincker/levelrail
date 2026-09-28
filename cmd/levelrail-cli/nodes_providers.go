package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"
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

NAME must be "hetzner", "digitalocean", "aws", "azure" or "gcp".

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

Lists every known cloud provider (hetzner, digitalocean, azure, gcp) and
whether a credential is currently stored for it.

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
	var provider, providerToken, secretAccessKey, sessionToken, region, roleARN string
	var useAmbient bool
	fs.StringVar(&provider, "provider", "", "hetzner, digitalocean, aws, azure or gcp (required)")
	fs.StringVar(&providerToken, "provider-token", "", "the provider's credential (an API token for hetzner/digitalocean, the AWS access key id for aws, a single-line JSON object for azure, a service account JSON key for gcp); prefer piping it on stdin or the interactive prompt instead, a flag value is visible in shell history and the process list")
	fs.StringVar(&secretAccessKey, "secret-access-key", "", "aws only: the secret access key matching --provider-token's access key id (required unless --use-ambient-credentials); prefer piping it on stdin or the interactive prompt instead, a flag value is visible in shell history and the process list")
	fs.StringVar(&sessionToken, "session-token", "", "aws only: a session token, for temporary credentials (optional)")
	fs.StringVar(&region, "region", "", "aws only: the region these credentials default to when a provision doesn't pick one explicitly (optional, default us-east-1)")
	fs.StringVar(&roleARN, "role-arn", "", "aws only: an IAM role ARN to assume via STS before use, so the stored key only needs sts:AssumeRole on this role rather than direct EC2 permissions (optional)")
	fs.BoolVar(&useAmbient, "use-ambient-credentials", false, "aws only: use this control plane's own AWS identity (env vars, shared config, or an EC2 instance profile) instead of a stored access key/secret")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesProvidersSetCredentialUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	switch {
	case provider == "aws" && useAmbient:
		// Nothing further to collect: the control plane resolves its own
		// ambient AWS identity, optionally narrowed by --role-arn.
	case provider == "aws":
		if providerToken == "" {
			read, err := readNodeProviderToken(stderr)
			if err != nil {
				return reportError(stdout, stderr, jsonOut, err)
			}
			providerToken = read
		}
		if secretAccessKey == "" {
			read, err := readNodeProviderSecret(stderr)
			if err != nil {
				return reportError(stdout, stderr, jsonOut, err)
			}
			secretAccessKey = read
		}
		if providerToken == "" || secretAccessKey == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--provider-token (access key id) and --secret-access-key are required for aws, or pass --use-ambient-credentials"))
		}
	default:
		if providerToken == "" {
			read, err := readNodeProviderToken(stderr)
			if err != nil {
				return reportError(stdout, stderr, jsonOut, err)
			}
			providerToken = read
		}
		if providerToken == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("a provider token is required: pass --provider-token, pipe it on stdin, or run interactively"))
		}
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	req := setNodeProviderCredentialRequest{Provider: provider, Token: providerToken}
	if provider == "aws" {
		req.SecretAccessKey, req.SessionToken, req.Region, req.RoleARN, req.UseAmbientCredentials = secretAccessKey, sessionToken, region, roleARN, useAmbient
	}
	updated, err := client.SetNodeProviderCredential(context.Background(), req)
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
  %[1]s nodes providers set-credential --provider NAME [--provider-token TOKEN] [flags]
  echo "$TOKEN" | %[1]s nodes providers set-credential --provider NAME

Stores (or replaces) a cloud provider's credential, encrypted the same way
every other integration credential in this platform is. It is never
echoed back.

For hetzner and digitalocean, --provider-token alone is the provider's
single bearer API token. AWS has no such single token: --provider-token
there is the access key id, paired with --secret-access-key (or pass
--use-ambient-credentials to use this control plane's own AWS identity
instead of a stored key, optionally narrowed with --role-arn either way).
For azure it is a single-line JSON object with tenant_id, client_id,
client_secret, subscription_id and resource_group (an Azure AD service
principal with Contributor access on that resource group); see docs for
an optional workload-identity-federation mode. For gcp it is a service
account JSON key's raw content, minified to one line; its project_id
field is used directly, no separate project flag exists.

--provider-token and --secret-access-key are both accepted for
scripting but their values end up in shell history and the process
list while the command runs. Prefer piping them on stdin (a
non-terminal stdin is read one line at a time, trimmed: the access key
id first if --provider-token was omitted, then the secret access key
if --secret-access-key was omitted), or omit them and run
interactively: a terminal stdin gets a no-echo prompt for each one in
turn instead.

Flags:
  --provider string             hetzner, digitalocean, aws, azure or gcp (required)
  --provider-token string        the provider's credential, or the AWS access key id, see above
  --secret-access-key string    aws only: the secret access key (required unless --use-ambient-credentials); stdin/prompt fallback like --provider-token
  --session-token string        aws only: a session token, for temporary credentials (optional)
  --region string                aws only: default region for these credentials (optional, default us-east-1)
  --role-arn string              aws only: an IAM role ARN to assume via STS before use (optional)
  --use-ambient-credentials      aws only: use this control plane's own AWS identity instead of a stored key
  --api-url string              control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string              named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                           print the updated provider as JSON to stdout, nothing else
  --output string                 output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string                  JMESPath expression to filter the result before printing
  -h, --help                      show this help
`, prog, envAPIURL, defaultAPIURL)
}

// readNodeProviderToken and readNodeProviderSecret are seams for tests;
// production callers get defaultReadNodeProviderToken/Secret.
var (
	readNodeProviderToken  = defaultReadNodeProviderToken
	readNodeProviderSecret = defaultReadNodeProviderSecret
)

// nodeProviderStdinReader is shared across both defaultReadNodeProviderToken
// and defaultReadNodeProviderSecret so a piped, non-terminal stdin can carry
// two lines (access key id, then secret access key) without the second read
// losing bytes the first read had already buffered from the pipe.
var nodeProviderStdinReader = bufio.NewReader(os.Stdin)

// defaultReadNodeProviderToken resolves the token when --provider-token
// was omitted: a piped, non-terminal stdin is read directly (one line,
// trimmed, the same "docker login --password-stdin" shape), a real
// terminal gets a no-echo prompt instead so the token is never displayed
// either way.
func defaultReadNodeProviderToken(stderr io.Writer) (string, error) {
	return readNodeProviderProtectedValue(stderr, "Provider token: ", "provider token")
}

// defaultReadNodeProviderSecret resolves --secret-access-key the same way
// defaultReadNodeProviderToken resolves --provider-token: a protected input
// path so the AWS secret access key never has to be passed as a flag, where
// it would be visible in shell history and the process list.
func defaultReadNodeProviderSecret(stderr io.Writer) (string, error) {
	return readNodeProviderProtectedValue(stderr, "Secret access key: ", "secret access key")
}

func readNodeProviderProtectedValue(stderr io.Writer, prompt, label string) (string, error) {
	fd := int(os.Stdin.Fd()) //nolint:gosec // a file descriptor always fits in int
	if !term.IsTerminal(fd) {
		line, err := nodeProviderStdinReader.ReadString('\n')
		if err != nil && line == "" {
			return "", newValidationError("read %s from stdin: %v", label, err)
		}
		return strings.TrimSpace(line), nil
	}
	_, _ = fmt.Fprint(stderr, prompt)
	b, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(stderr)
	if err != nil {
		return "", newValidationError("read %s: %v", label, err)
	}
	return strings.TrimSpace(string(b)), nil
}
