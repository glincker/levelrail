package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"golang.org/x/term"
)

// runTokensList implements "tokens list": GET /api/v1/auth/tokens,
// authenticated with a freshly established session, the same reason
// "tokens create" is (tokens_create.go's own doc comment). Never
// includes a token secret, matching the server's own contract.
func runTokensList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" tokens list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var username, password, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&username, "username", "", "admin username (prompted if omitted)")
	fs.StringVar(&password, "password", "", "admin password (prompted without echo if omitted)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print tokens as a JSON array to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	readPassword := func() (string, error) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(b), err
	}
	resolvedUsername, resolvedPassword, err := resolveLoginCredentials(username, password, stdin, stderr, readPassword)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	sessionClient, err := newAuthSessionClient(resolveAPIURL(apiURLFlag, lookupEnv, prog))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	ctx := context.Background()
	if _, err := sessionClient.Login(ctx, resolvedUsername, resolvedPassword); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("log in as %q: %w", resolvedUsername, err))
	}

	tokens, err := sessionClient.ListTokens(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, tokens); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printTokensTable(stdout, tokens)
	return exitOK
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
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tABILITIES\tCREATED\tREVOKED")
	for _, t := range tokens {
		revoked := "no"
		if t.RevokedAt != nil {
			revoked = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%v\t%s\t%s\n", t.ID, t.Name, t.Abilities, t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), revoked)
	}
	_ = tw.Flush()
}

func tokensListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens list [flags]

Lists every API token (never a secret value). Requires a live session:
--username/--password (prompted if omitted).

Flags:
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --json                          print tokens as a JSON array to stdout, nothing else
  -h, --help                    show this help
`, prog, envAPIURL, defaultAPIURL)
}
