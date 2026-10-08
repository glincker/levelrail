package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runTokensList implements "tokens list": GET /api/v1/auth/tokens,
// authenticated with a freshly established session, the same reason
// "tokens create" is (tokens_create.go's own doc comment). Never
// includes a token secret, matching the server's own contract.
func runTokensList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "tokens list", "print tokens as a JSON array to stdout and nothing else", stderr)
	tokenFlagP := fs.String("token", "", "API token with root ability (skips the password prompt; overrides "+envAPIToken+" and the credentials file)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	jsonOut := *jsonOutP
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, *queryFlagP}

	ctx := context.Background()
	if tok := listTokenAuth(*tokenFlagP, *usernameP, *passwordP, *profileFlagP, prog, lookupEnv); tok != "" {
		client := NewClient(resolveAPIURL(*apiURLFlagP, lookupEnv, prog, resolveProfile(*profileFlagP, lookupEnv)), tok)
		raw, err := client.ListTokensRaw(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens: %w", err))
		}
		var tokens []tokenResource
		if err := json.Unmarshal(raw, &tokens); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens: decode: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, tokens, func() { printTokensTable(stdout, tokens) })
	}
	sessionClient, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	tokens, err := sessionClient.ListTokens(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, tokens, func() { printTokensTable(stdout, tokens) })
}

// listTokenAuth returns the bearer token to list with, or "" to fall back to
// a session login: an explicit --token wins, then a stored token unless the
// caller passed session credentials.
func listTokenAuth(flagToken, username, password, profileFlag, prog string, lookupEnv func(string) (string, bool)) string {
	if flagToken != "" {
		return flagToken
	}
	if username != "" || password != "" {
		return ""
	}
	return resolveToken("", lookupEnv, prog, resolveProfile(profileFlag, lookupEnv))
}

// printTokensTable prints a compact, aligned table of tokens, never a
// secret value, the same printAppsTable/printDatabasesTable shape
// output.go's own helpers already establish for other list commands.
func printTokensTable(out io.Writer, tokens []tokenResource) {
	if len(tokens) == 0 {
		_, _ = fmt.Fprintln(out, "no tokens")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tAGENT\tABILITIES\tCREATED\tREVOKED")
	for _, t := range tokens {
		revoked := "no"
		if t.RevokedAt != nil {
			revoked = "yes"
		}
		agent := "-"
		if t.Agent != nil {
			agent = t.Agent.Name
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%s\t%s\n", t.ID, t.Name, agent, t.Abilities, t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), revoked)
	}
	_ = tw.Flush()
}

func tokensListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens list [flags]

Lists every API token (never a secret value). Authenticates with a stored
or --token API token holding root, else a session login (--username/--password,
prompted if omitted).

Flags:
  --token string             API token with root ability (skips the password prompt)
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print tokens as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}
