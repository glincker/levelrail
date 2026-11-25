package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsDeploysPin implements "apps deploys pin|unpin <name> <deploy-id>".
func runAppsDeploysPin(prog string, unpin bool, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "pin"
	if unpin {
		verb = "unpin"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys "+verb, "print the deploy attempt as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysPinUsage(prog, verb)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps deploys "+verb, "an app name and a deploy attempt id", 2)
	if !ok {
		return exitUsage
	}
	name, deployID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var attempt apiclient.DeployAttemptResource
	var err error
	if unpin {
		attempt, err = client.UnpinDeploy(context.Background(), name, deployID)
	} else {
		attempt, err = client.PinDeploy(context.Background(), name, deployID)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s deploy %q of app %q: %w", verb, deployID, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, attempt, func() {
		state := "pinned"
		if !attempt.Pinned {
			state = "not pinned"
		}
		_, _ = fmt.Fprintf(stdout, "deploy %s of app %q is now %s\n", attempt.ID, name, state)
	})
}

func appsDeploysPinUsage(prog, verb string) string {
	what := "Keeps a deploy and its image out of image retention and janitor cleanup, so it stays a known good rollback target."
	if verb == "unpin" {
		what = "Releases a pin so retention and the janitor may remove the deploy and its image again."
	}
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys %[5]s <name> <deploy-id> [flags]

%[6]s
Rolling back to a pinned deploy works as for any other deploy.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the deploy attempt as JSON to stdout, nothing else
  --output string          output format: json, ndjson, yaml, table, wide, text or name (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL, verb, what)
}
