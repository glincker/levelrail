package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runDomainsWWWPlan implements "domains www-plan <app> <domain>".
func runDomainsWWWPlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains www-plan", "print the plan as JSON to stdout and nothing else", stderr)
	var mode, canonical string
	var noRedirect bool
	fs.StringVar(&mode, "mode", "", "both, apex_only or www_only (default: the domains.www_mode setting)")
	fs.StringVar(&canonical, "canonical", "", "apex or www (default: the domains.canonical setting)")
	fs.BoolVar(&noRedirect, "no-redirect", false, "preview serving both hosts without the 308 redirect")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsWWWPlanUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains www-plan", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	plan, err := client.DomainWWWPlan(context.Background(), rest[0], rest[1], mode, canonical, noRedirect)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("www plan for %q in app %q: %w", rest[1], rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, plan, func() {
		_, _ = fmt.Fprintf(stdout, "%s: mode %s, canonical %s\n", plan.Domain, plan.Mode, plan.Canonical)
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		for _, r := range plan.Records {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%ds\t%s\n", r.Name, r.Type, r.Value, r.TTLSeconds, r.Outcome)
		}
		_ = tw.Flush()
		if plan.Redirect != nil {
			_, _ = fmt.Fprintf(stdout, "redirect: %s -> %s (308)\n", plan.Redirect.From, plan.Redirect.To)
		}
		_, _ = fmt.Fprintf(stdout, "www attached to app: %s\n", wwwYesNo(plan.WWWAttached))
		_, _ = fmt.Fprintf(stdout, "www DNS points here: %s\n", wwwYesNo(plan.WWWPointsHere))
		if !plan.WWWAttached {
			_, _ = fmt.Fprintf(stdout, "next: %s domains www-apply %s %s --attach --yes\n", prog, rest[0], rest[1])
		}
		if plan.Message != "" {
			_, _ = fmt.Fprintf(stdout, "note: %s\n", plan.Message)
		}
	})
}

func wwwYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func domainsWWWPlanUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains www-plan <app> <domain> [--mode both|apex_only|www_only] [--canonical apex|www] [--no-redirect] [flags]

Previews the apex and www DNS records for a bare domain and the 308 redirect
between the two hosts, plus whether www is attached to the app and whether
public DNS for it points at this server. Read only: no DNS record is written.

Flags:
  --mode string            both, apex_only or www_only (default: the domains.www_mode setting)
  --canonical string       apex or www (default: the domains.canonical setting)
  --no-redirect            preview serving both hosts without the 308 redirect
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                   print the plan as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
