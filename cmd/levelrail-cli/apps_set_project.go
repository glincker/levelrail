package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsSetProject implements "apps set-project <name> <project-id>":
// PUT /api/v1/apps/{name}/project. The app's current project_id is
// already visible on "apps get <name>", the same reasoning
// runAppsSetEnvironment's own doc comment gives for having no separate
// "get" subcommand here.
func runAppsSetProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps set-project", "print the updated app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsSetProjectUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest, ok := requireArgs(fs, stderr, prog, "apps set-project", "an app name and a project id", 2)
	if !ok {
		return exitUsage
	}
	name, projectID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetAppProject(context.Background(), name, projectID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set project for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "app %q moved into project %q\n", name, projectID)
	})
}

func appsSetProjectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps set-project <name> <project-id> [flags]

Moves an app into a project (create one first via "%[1]s apps projects
create"). Purely organizational, doesn't change how the app runs.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsClearProject implements "apps clear-project <name>": PUT
// /api/v1/apps/{name}/project with an empty project_id.
func runAppsClearProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps clear-project", "print the updated app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsClearProjectUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	name, ok := requireOneArg(fs, stderr, prog, "apps clear-project", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetAppProject(context.Background(), name, "")
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear project for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "app %q removed from its project\n", name)
	})
}

func appsClearProjectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps clear-project <name> [flags]

Removes an app's project assignment.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
