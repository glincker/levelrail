package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runEnvironmentsSetDefault(prog, label string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label+" set-default", "print the updated project as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, environmentsSetDefaultUsage(prog, label)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, label+" set-default", "a project id and an environment id or name", 2)
	if !ok {
		return exitUsage
	}
	projectID, env := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	envID := env
	envs, err := client.ListEnvironments(context.Background(), projectID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list environments of project %q: %w", projectID, err))
	}
	for _, e := range envs {
		if e.ID == env {
			break
		}
		if strings.EqualFold(e.Name, env) {
			envID = e.ID
		}
	}
	p, err := client.SetProjectDefaultEnvironment(context.Background(), projectID, envID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set default environment of project %q: %w", projectID, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() {
		_, _ = fmt.Fprintf(stdout, "project %q default environment is now %s\n", projectID, p.DefaultEnvironmentID)
	})
}

func environmentsSetDefaultUsage(prog, label string) string {
	return fmt.Sprintf(`Usage:
  %[1]s %[5]s set-default <project-id> <environment-id|name> [flags]

Makes one of a project's environments its default. An app or database
created in the project without naming an environment joins it, resolved by
the control plane. The previous default is cleared.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated project as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL, label)
}

func runAppsProjectsBackfillDefaultEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps projects backfill-default-env", "print the result as JSON to stdout and nothing else", stderr)
	yes := fs.Bool("yes", false, "apply the change; without it only the plan is shown")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsBackfillUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if len(fs.Args()) != 0 {
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.BackfillDefaultEnvironments(context.Background(), !*yes)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("backfill default environments: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		verb := "would"
		if !res.DryRun {
			verb = "did"
		}
		_, _ = fmt.Fprintf(stdout, "%d project(s) without a default environment (name %q)\n", len(res.Projects), res.Name)
		for _, p := range res.Projects {
			_, _ = fmt.Fprintf(stdout, "  %s %s: %s %s\n", p.ProjectID, p.Name, verb, p.Action)
		}
		if res.DryRun && len(res.Projects) > 0 {
			_, _ = fmt.Fprintf(stdout, "dry run, nothing changed; rerun with --yes to apply\n")
		}
	})
}

func appsProjectsBackfillUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects backfill-default-env [--yes] [flags]

Gives every project without a default environment one, named by the
projects.default_environment_name setting (an existing environment with that
name is promoted instead). Instance admins only. Without --yes it is a dry
run that only lists what would change.

Flags:
  --yes                      apply the change
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
