package main

import (
	"context"
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
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP := sessionFlagSet(prog, "tokens list", "print tokens as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	jsonOut := *jsonOutP

	ctx := context.Background()
	sessionClient, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	tokens, err := sessionClient.ListTokens(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, tokens, func() { printTokensTable(stdout, tokens) })
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
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print tokens as a JSON array to stdout, nothing else
  -h, --help                    show this help
`, prog, envAPIURL, defaultAPIURL)
}
