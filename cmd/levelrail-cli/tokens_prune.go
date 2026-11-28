package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// parseOlderThanDays accepts 30, 30d or 0d.
func parseOlderThanDays(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(raw), "d"))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("--older-than must be a number of days such as 30d")
	}
	return n, nil
}

// runTokensPrune deletes revoked and expired tokens past a window; it previews first and needs a root API token.
func runTokensPrune(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" tokens prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	olderThan := fs.String("older-than", strconv.Itoa(defaultTokenCleanupDays)+"d", "delete revoked and expired tokens that ended more than this long ago, in days")
	tokenFlag := fs.String("token", "", "API token with root ability (overrides "+apiTokenEnvHelp+" and the credentials file)")
	apiURLFlag := fs.String("api-url", "", "control plane API base URL (overrides "+apiURLEnvHelp+" and the credentials file, default "+defaultAPIURL+")")
	profileFlag := fs.String("profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	dryRun := fs.Bool("dry-run", false, "only show how many tokens would be deleted")
	yes := fs.Bool("yes", false, "skip the confirmation question")
	jsonOut := fs.Bool("json", false, "print a JSON result object to stdout and nothing else")
	outputFlag, queryFlag := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensPruneUsage(prog)) }

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
	days, derr := parseOlderThanDays(*olderThan)
	if derr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, derr)
		return exitValidation
	}
	ctx := context.Background()
	profile := resolveProfile(*profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(*apiURLFlag, lookupEnv, prog, profile), resolveToken(*tokenFlag, lookupEnv, prog, profile))
	preview, err := client.PruneTokens(ctx, days, true)
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, fmt.Errorf("prune tokens: %w", err))
	}
	res := preview
	if n := preview.Revoked + preview.Expired; n > 0 && !*dryRun {
		_, _ = fmt.Fprintf(stderr, "%d revoked and %d expired tokens older than %d days will be deleted\n", preview.Revoked, preview.Expired, days)
		if err := confirmAction(stdin, stderr, fmt.Sprintf("Delete %d tokens", n), *yes, "--yes"); err != nil {
			return reportError(stdout, stderr, *jsonOut, err)
		}
		if res, err = client.PruneTokens(ctx, days, false); err != nil {
			return reportError(stdout, stderr, *jsonOut, fmt.Errorf("prune tokens: %w", err))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, outputFlags{format, *queryFlag}, res, func() {
		verb := "deleted"
		if res.DryRun {
			verb = "would delete"
		}
		_, _ = fmt.Fprintf(stdout, "%s %d revoked and %d expired tokens\n", verb, res.Revoked, res.Expired)
	})
}

func tokensPruneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens prune [--older-than 30d] [--dry-run] [--yes] [flags]

Permanently deletes revoked tokens and expired tokens that ended more than
the given number of days ago (default %[4]dd). A token that still works is
never touched. It previews first, prints the counts and asks to confirm
unless --yes is given. Needs an API token with root ability.

Flags:
  --older-than string        days, for example 30d (default %[4]dd)
  --dry-run                  only show the counts
  --yes                      skip the confirmation question
  --token string             API token with root ability
  --api-url string           control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string           named credentials profile to read
  --json                     print a JSON result object to stdout, nothing else
  --output string            output format: json, ndjson, yaml, table, wide, text or name
  --query string             JMESPath expression to filter the result before printing
  -h, --help                 show this help
`, prog, apiURLEnvHelp, defaultAPIURL, defaultTokenCleanupDays)
}
