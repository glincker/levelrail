package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runTokensUnused lists live tokens idle past the tokens.unused_alert_days setting; it never revokes.
func runTokensUnused(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "tokens unused", "print the unused tokens as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensUnusedUsage(prog)) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
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
	client, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	res, err := client.ListUnusedTokens(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list unused tokens: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		switch {
		case res.AlertDays == 0:
			_, _ = fmt.Fprintf(stdout, "the unused token alert is off: set tokens.unused_alert_days with \"%s settings runtime set\"\n", prog)
		case len(res.Items) == 0:
			_, _ = fmt.Fprintf(stdout, "no token has been idle for %d days or more\n", res.AlertDays)
		default:
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tNAME\tABILITIES\tCREATED\tLAST USED")
			for _, t := range res.Items {
				last := "never"
				if t.LastUsedAt != nil {
					last = t.LastUsedAt.Format("2006-01-02")
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%v\t%s\t%s\n", t.ID, t.Name, t.Abilities, t.CreatedAt.Format("2006-01-02"), last)
			}
			_ = tw.Flush()
			_, _ = fmt.Fprintf(stdout, "revoke one with \"%s tokens revoke <id>\"; nothing is revoked automatically\n", prog)
		}
	})
}

func tokensUnusedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens unused [flags]

Lists live tokens not used for tokens.unused_alert_days days (a never-used
token counts from its creation). The alert is off while that setting is 0.
Read only: nothing is revoked, and no secret is ever shown. Requires a
username and password (prompted if omitted).

Flags:
  --username string   admin username (prompted if omitted)
  --password string   admin password (prompted without echo if omitted)
  --api-url string    control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the unused tokens as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, apiURLEnvHelp, defaultAPIURL)
}
