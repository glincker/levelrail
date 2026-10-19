package main

import (
	"context"
	"fmt"
	"io"
)

func runAppsAlertsTemplate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps alerts template", "print the result as JSON to stdout and nothing else", stderr)
	profile := fs.String("profile-type", "web", "app shape: web, worker or database")
	channel := fs.String("channel", "", "notification channel id the new rules notify")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsAlertsTemplateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps alerts template", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ApplyAlertTemplate(context.Background(), appName, *profile, *channel)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply alert template to app %q: %w", appName, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		for _, r := range res.Created {
			_, _ = fmt.Fprintf(stdout, "created  %s (%s)\n", r.Name, r.Kind)
		}
		for _, n := range res.Skipped {
			_, _ = fmt.Fprintf(stdout, "skipped  %s (already exists)\n", n)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsAlertsTemplateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps alerts template <app> [--profile-type web|worker|database] [--channel ID] [flags]

Creates a sensible alert rule set for the app in one step. A web app gets
crash loop, 5xx rate, p95 latency, CPU and a 99.9%% availability SLO rules;
workers and databases get crash loop and CPU. Rules whose name already
exists are skipped, so running it twice is safe.

Flags:
  --profile-type string  app shape: web, worker or database (default web)
  --channel string       notification channel id the new rules notify
  --token string         API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                 print the result as JSON to stdout, nothing else
  --output string        output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help             show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
