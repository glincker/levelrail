package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const forkApprovalWarning = "Approving deploys code from a fork. It runs with this app's environment variables and secrets, so read the change first. Only the current commit deploys; a later push from the fork needs a new approval."

func runAppsPreviewsLimits(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps previews limits", "print the policy as JSON to stdout and nothing else", stderr)
	onLimit := fs.String("on-limit", "", "when the preview cap is full: evict_oldest or reject")
	allowForks := fs.Bool("allow-forks", false, "allow previews for pull requests from forks (they receive the app's secrets)")
	ttlHours := fs.Int("ttl-hours", 0, "hours a preview lives without updates (0 uses the platform default)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsLimitsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if fs.NArg() == 0 {
		overview, err := client.ListAllPreviews(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get preview limits: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, overview.Limits, func() {
			_, _ = fmt.Fprintf(stdout, "live previews: %d (platform limit %s)\nper-app limit: %s\n",
				overview.Limits.LiveTotal, limitLabel(overview.Limits.MaxTotal), limitLabel(overview.Limits.MaxPerApp))
		})
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps previews limits", "app name")
	if !ok {
		return exitUsage
	}

	var req apiclient.SetPreviewPolicyRequest
	changed := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "on-limit":
			req.OnLimit, changed = onLimit, true
		case "allow-forks":
			req.AllowForkPreviews, changed = allowForks, true
		case "ttl-hours":
			req.TTLHours, changed = ttlHours, true
		}
	})

	var (
		policy apiclient.PreviewPolicyResource
		err    error
	)
	if changed {
		policy, err = client.SetPreviewPolicy(ctx, appName, req)
	} else {
		policy, err = client.GetPreviewPolicy(ctx, appName)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview limits for app %q: %w", appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, policy, func() { printPreviewPolicy(stdout, appName, policy) })
}

func limitLabel(n int) string {
	if n <= 0 {
		return "unlimited"
	}
	return strconv.Itoa(n)
}

func printPreviewPolicy(out io.Writer, appName string, p apiclient.PreviewPolicyResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "app\t%s\n", appName)
	_, _ = fmt.Fprintf(tw, "live previews (this app)\t%d of %s\n", p.LiveCount, limitLabel(p.MaxPerApp))
	_, _ = fmt.Fprintf(tw, "live previews (platform)\t%d of %s\n", p.LiveTotal, limitLabel(p.MaxTotal))
	_, _ = fmt.Fprintf(tw, "when the limit is full\t%s\n", p.OnLimit)
	_, _ = fmt.Fprintf(tw, "fork pull requests\t%s\n", map[bool]string{true: "deploy automatically (they receive secrets)", false: "wait for approval"}[p.AllowForkPreviews])
	_, _ = fmt.Fprintf(tw, "ttl\t%d hours\n", p.EffectiveTTLHours)
	_ = tw.Flush()
}

func appsPreviewsLimitsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews limits [flags]              platform-wide preview usage and caps
  %[1]s apps previews limits <app-name> [flags]   show or change one app's preview policy

Caps come from APP_PREVIEW_ENV_MAX_PER_APP and APP_PREVIEW_ENV_MAX_TOTAL on
the control plane. Give any of the policy flags to change the app's policy.

Flags:
  --on-limit string       when the cap is full: evict_oldest (default) or reject
  --allow-forks           deploy pull requests from forks without approval (they receive the app's secrets)
  --ttl-hours int         hours a preview lives without updates (0 uses the platform default)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsPreviewsApprove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps previews approve", "print the result as JSON to stdout and nothing else", stderr)
	yes := fs.Bool("yes", false, "confirm that you reviewed the fork's changes and accept the security implication")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsApproveUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	positional := fs.Args()
	if len(positional) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps previews approve requires an app name and a pr number\n\n", prog)
		_, _ = fmt.Fprint(stderr, appsPreviewsApproveUsage(prog))
		return exitUsage
	}
	appName := positional[0]
	prNumber, err := strconv.Atoi(positional[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("pr number must be an integer"))
	}
	if !*yes {
		return reportError(stdout, stderr, jsonOut, newValidationError(forkApprovalWarning+" Re-run with --yes to approve."))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.ApprovePreviewEnvironment(context.Background(), appName, prNumber)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("approve preview for app %q pr #%d: %w", appName, prNumber, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "preview for app %q pr #%d approved and deploying\n", appName, prNumber)
	})
}

func appsPreviewsApproveUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews approve <app-name> <pr-number> --yes [flags]

Deploys a held fork pull request once. %[5]s

Flags:
  --yes                   confirm the above (required)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, forkApprovalWarning)
}
