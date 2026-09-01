package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsSetEnvironment implements "apps set-environment <name>
// <environment-id>": PUT /api/v1/apps/{name}/environment. The app's
// current environment_id is already visible on "apps get <name>", so
// unlike apps log-drain there is no separate "get" subcommand here.
func runAppsSetEnvironment(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps set-environment", "print the updated app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsSetEnvironmentUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps set-environment", "an app name and an environment id", 2)
	if !ok {
		return exitUsage
	}
	name, environmentID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetAppEnvironment(context.Background(), name, environmentID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set environment for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "app %q tagged with environment %q\n", name, environmentID)
	})
}

func appsSetEnvironmentUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps set-environment <name> <environment-id> [flags]

Tags an app with an environment (create one first via "%[1]s apps
environments create").

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsClearEnvironment implements "apps clear-environment <name>":
// PUT /api/v1/apps/{name}/environment with an empty environment_id.
func runAppsClearEnvironment(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps clear-environment", "print the updated app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsClearEnvironmentUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps clear-environment", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetAppEnvironment(context.Background(), name, "")
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear environment for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "app %q untagged from its environment\n", name)
	})
}

func appsClearEnvironmentUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps clear-environment <name> [flags]

Removes an app's environment tag.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
