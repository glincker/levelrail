package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runOnboarding implements "onboarding status|skip|reset": the signed-in
// user's own first-run flow, from /api/v1/me/onboarding.
func runOnboarding(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, onboardingUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	sub := args[0]
	switch sub {
	case "status", "skip", "reset":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown onboarding command %q\n\n%s", prog, sub, onboardingUsage(prog))
		return exitUsage
	}

	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "onboarding "+sub, "print the onboarding state as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, onboardingUsage(prog)) }
	confirm := fs.Bool("yes", false, "confirm a reset (required for onboarding reset)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if sub == "reset" && !*confirm {
		_, _ = fmt.Fprintf(stderr, "%s: onboarding reset restarts your first-run flow; pass --yes to confirm\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	var (
		o   apiclient.MeOnboarding
		err error
	)
	switch sub {
	case "skip":
		o, err = client.SetMeOnboarding(ctx, "dismissed", "")
	case "reset":
		o, err = client.ResetMeOnboarding(ctx)
	default:
		o, err = client.GetMeOnboarding(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("onboarding %s: %w", sub, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, o, func() { printMeOnboarding(stdout, o) })
}

func printMeOnboarding(out io.Writer, o apiclient.MeOnboarding) {
	_, _ = fmt.Fprintf(out, "State: %s\nCurrent step: %s\nMilestone: %s\nNext milestone: %s\nEnabled: %t\n",
		o.State, dashIfEmpty(o.CurrentStep), dashIfEmpty(o.Milestone), dashIfEmpty(o.NextMilestone), o.Enabled)
}

func onboardingUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s onboarding status [flags]       your first-run flow state, current step and install milestone
  %[1]s onboarding skip [flags]         dismiss the first-run flow
  %[1]s onboarding reset --yes [flags]  restart the first-run flow from the beginning

Flags:
  --yes               confirm a reset
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the state as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
