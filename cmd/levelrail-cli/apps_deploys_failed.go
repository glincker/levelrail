package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// runAppsDeploysFailed implements "apps deploys failed [--since 24h]".
func runAppsDeploysFailed(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys failed", "print the failed deploys as a JSON array to stdout and nothing else", stderr)
	var since string
	fs.StringVar(&since, "since", "", "only deploys that failed within this window, a Go duration such as 24h (default: server default)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysFailedUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys failed takes no arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if since != "" {
		if d, err := time.ParseDuration(since); err != nil || d <= 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("--since must be a positive duration such as 24h"))
		}
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	failed, err := client.ListFailedDeploys(context.Background(), since)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list failed deploys: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, failed, func() { printFailedDeploysTable(stdout, failed) })
}

func printFailedDeploysTable(out io.Writer, failed []failedDeployResource) {
	if len(failed) == 0 {
		_, _ = fmt.Fprintln(out, "no failed deploys")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tDEPLOY ID\tIMAGE\tLAST GOOD IMAGE\tSTARTED\tERROR")
	for _, f := range failed {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", f.ServiceName, f.ID, f.Image, dashIfEmpty(f.LastGoodImage), f.StartedAt.Format(time.RFC3339), dashIfEmpty(f.Error))
	}
	_ = tw.Flush()
}

func appsDeploysFailedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys failed [--since 24h] [flags]

Lists every app's latest failed deploy attempt in the window, with the
image of its newest good deploy so a rollback target is one copy away
("apps rollback <name> --image IMAGE").

Flags:
  --since string          only deploys that failed within this window, e.g. 24h (default: server default)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the failed deploys as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
