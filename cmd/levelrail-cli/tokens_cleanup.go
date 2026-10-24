package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

const defaultTokenCleanupDays = 90

// runTokensCleanup removes revoked tokens past a window. It needs a root API token, not a session.
func runTokensCleanup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" tokens cleanup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	days := fs.Int("older-than-days", defaultTokenCleanupDays, "remove revoked tokens revoked more than this many days ago (0 removes every revoked token)")
	tokenFlag := fs.String("token", "", "API token with root ability (overrides "+envAPIToken+" and the credentials file)")
	apiURLFlag := fs.String("api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	profileFlag := fs.String("profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	yes := fs.Bool("yes", false, "confirm removing every revoked token (required with --older-than-days 0)")
	jsonOut := fs.Bool("json", false, "print a JSON result object to stdout and nothing else")
	outputFlag, queryFlag := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensCleanupUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, ferr := resolveOutputFormat(*jsonOut, *outputFlag)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	if *days < 0 {
		_, _ = fmt.Fprintf(stderr, "%s: --older-than-days must be 0 or more\n", prog)
		return exitValidation
	}
	if *days == 0 && !*yes {
		_, _ = fmt.Fprintf(stderr, "%s: --older-than-days 0 removes every revoked token, add --yes to confirm\n", prog)
		return exitValidation
	}
	profile := resolveProfile(*profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(*apiURLFlag, lookupEnv, prog, profile), resolveToken(*tokenFlag, lookupEnv, prog, profile))
	res, err := client.CleanupTokens(context.Background(), *days)
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, fmt.Errorf("clean up tokens: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, outputFlags{format, *queryFlag}, res, func() {
		_, _ = fmt.Fprintf(stdout, "removed %d revoked tokens\n", res.Deleted)
	})
}

func tokensCleanupUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens cleanup [--older-than-days N] [--yes] [flags]

Permanently removes revoked API tokens that were revoked more than N days
ago (default %[4]d; 0 removes every revoked token). Active tokens are never
touched. Needs an API token with root ability.

Flags:
  --older-than-days int      window in days (default %[4]d)
  --yes                      confirm --older-than-days 0 (removes every revoked token)
  --yes                      confirm --older-than-days 0 (removes every revoked token)
  --token string             API token with root ability
  --api-url string          control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string           named credentials profile to read
  --json                     print a JSON result object to stdout, nothing else
  --output string            output format: json, table, or text
  --query string             JMESPath expression to filter the result before printing
  -h, --help                 show this help
`, prog, envAPIURL, defaultAPIURL, defaultTokenCleanupDays)
}
