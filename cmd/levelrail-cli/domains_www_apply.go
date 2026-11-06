package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runDomainsWWWApply implements "domains www-apply <app> <domain>".
func runDomainsWWWApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains www-apply", "print the result as JSON to stdout and nothing else", stderr)
	var mode, canonical string
	var replace, yes, noRedirect bool
	fs.StringVar(&mode, "mode", "", "both, apex_only or www_only (default: the domains.www_mode setting)")
	fs.StringVar(&canonical, "canonical", "", "apex or www (default: the domains.canonical setting)")
	fs.BoolVar(&replace, "replace", false, "overwrite conflicting records")
	fs.BoolVar(&noRedirect, "no-redirect", false, "serve both hosts without the 308 redirect")
	fs.BoolVar(&yes, "yes", false, "write the records; without it only the plan is shown")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsWWWApplyUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains www-apply", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if !yes {
		plan, err := client.DomainWWWPlan(ctx, rest[0], rest[1], mode, canonical, noRedirect)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("www plan for %q in app %q: %w", rest[1], rest[0], err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, plan, func() {
			for _, r := range plan.Records {
				_, _ = fmt.Fprintf(stdout, "%s %s -> %s (%s)\n", r.Type, r.Name, r.Value, r.Outcome)
			}
			_, _ = fmt.Fprintf(stdout, "nothing was written; re-run with --yes to apply\n")
		})
	}
	res, err := client.DomainWWWApply(ctx, rest[0], rest[1], mode, canonical, replace, true, noRedirect)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("www apply for %q in app %q: %w", rest[1], rest[0], err))
	}
	code := writeScheduledTaskResult(stdout, stderr, of, res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		for _, r := range res.Records {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Name, r.Type, r.Value, r.Result, r.Message)
		}
		_ = tw.Flush()
		if res.Redirect != nil {
			state := "applied"
			if !res.RedirectApplied {
				state = "not applied: " + res.RedirectMessage
			}
			_, _ = fmt.Fprintf(stdout, "redirect %s -> %s (308): %s\n", res.Redirect.From, res.Redirect.To, state)
		}
		for _, u := range res.Undo {
			_, _ = fmt.Fprintf(stdout, "undo: %s %s %s %s\n", u.Kind, u.Name, u.RecordType, u.Value)
		}
		if res.RunID != "" {
			_, _ = fmt.Fprintf(stdout, "run: %s (undo it with POST /api/v1/domains/automation/runs/%s/undo)\n", res.RunID, res.RunID)
		}
		if res.Message != "" {
			_, _ = fmt.Fprintf(stdout, "note: %s\n", res.Message)
		}
	})
	if code == 0 && !res.Applied {
		return exitCheckFailed
	}
	return code
}

func domainsWWWApplyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains www-apply <app> <domain> [--mode both|apex_only|www_only] [--canonical apex|www] [--replace] [--no-redirect] [--yes] [flags]

Writes the apex and www DNS records shown by www-plan, and the 308 redirect
when both hosts are attached to the app. Without --yes only the plan is
printed. Conflicting records are left alone unless --replace is given. The
first failed write stops the run and the result lists what was and was not done.

Flags:
  --mode string            both, apex_only or www_only (default: the domains.www_mode setting)
  --canonical string       apex or www (default: the domains.canonical setting)
  --replace                overwrite conflicting records
  --no-redirect            serve both hosts without the 308 redirect
  --yes                    write the records; without it only the plan is shown
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                   print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
