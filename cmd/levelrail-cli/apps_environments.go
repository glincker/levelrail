package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsEnvironments dispatches "apps environments <verb> [flags]" to
// one of create/list/delete, the CLI counterpart of
// internal/api/environments.go's own routes. An environment is a
// staging/production-style label scoped to one project, tagged onto an
// app via "apps set-environment".
func runAppsEnvironments(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsEnvironmentsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsEnvironmentsUsage(prog))
		return exitOK
	case "create":
		return runAppsEnvironmentsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runAppsEnvironmentsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsEnvironmentsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "env-get":
		return runAppsEnvironmentsEnvGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "env-set":
		return runAppsEnvironmentsEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps environments subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsEnvironmentsUsage(prog))
		return exitUsage
	}
}

func appsEnvironmentsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments create <project-id> --name NAME [flags]   create an environment under a project
  %[1]s apps environments list <project-id> [flags]                     list a project's environments
  %[1]s apps environments delete <id> [flags]                           delete an environment
  %[1]s apps environments env-get <id> [flags]                          show an environment's shared env vars
  %[1]s apps environments env-set <id> --var KEY=VALUE [flags]      replace an environment's shared env vars

Tag an app with an environment via "%[1]s apps set-environment". An
environment's shared env vars sit between its project's own shared env
vars and a tagged app's own env: overriding the project's, overridden by
the app's.

Run "%[1]s apps environments <subcommand> -h" for a subcommand's own
flags.
`, prog)
}

func runAppsEnvironmentsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps environments create", "print the created environment as JSON to stdout and nothing else", stderr)
	var name string
	fs.StringVar(&name, "name", "", "environment name, e.g. staging or production (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsEnvironmentsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	projectID, ok := requireOneArg(fs, stderr, prog, "apps environments create", "project id")
	if !ok {
		return exitUsage
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	created, err := client.CreateEnvironment(context.Background(), projectID, createEnvironmentRequest{Name: name})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create environment %q for project %q: %w", name, projectID, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, created, func() {
		_, _ = fmt.Fprintf(stdout, "environment %q created (id %s) for project %q\n", created.Name, created.ID, projectID)
	})
}

func appsEnvironmentsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments create <project-id> --name NAME [flags]

Creates a new environment under a project.

Flags:
  --name string            environment name, e.g. staging or production (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the created environment as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsEnvironmentsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps environments list", "print environments as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsEnvironmentsListUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	projectID, ok := requireOneArg(fs, stderr, prog, "apps environments list", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	envs, err := client.ListEnvironments(context.Background(), projectID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list environments for project %q: %w", projectID, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, envs, func() { printEnvironmentsTable(stdout, envs) })
}

func printEnvironmentsTable(out io.Writer, envs []environmentResource) {
	if len(envs) == 0 {
		_, _ = fmt.Fprintln(out, "no environments")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCREATED")
	for _, e := range envs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", e.ID, e.Name, e.CreatedAt)
	}
	_ = tw.Flush()
}

func appsEnvironmentsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments list <project-id> [flags]

Lists a project's environments.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print environments as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsEnvironmentsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps environments delete", "print {} to stdout on success instead of a plain confirmation", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsEnvironmentsDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps environments delete", "environment id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	if err := client.DeleteEnvironment(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete environment %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, map[string]any{}, func() {
		_, _ = fmt.Fprintf(stdout, "environment %q deleted\n", id)
	})
}

func appsEnvironmentsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments delete <id> [flags]

Deletes an environment. Any app tagged with it survives, simply
untagged again.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {} to stdout on success instead of a plain confirmation
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsEnvironmentsEnvGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps environments env-get", "print the env vars as a JSON object to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsEnvironmentsEnvGetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps environments env-get", "environment id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	vars, err := client.GetEnvironmentEnv(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get env vars for environment %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, vars, func() { printEnvVarsTable(stdout, vars) })
}

func appsEnvironmentsEnvGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments env-get <id> [flags]

Shows an environment's shared env vars: overrides its project's own
shared env vars, and is itself overridden by any app tagged with it.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the env vars as a JSON object to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsEnvironmentsEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps environments env-set", "print the resulting env vars as a JSON object to stdout and nothing else", stderr)
	vars := stringMapFlag{}
	fs.Var(vars, "var", "shared env var as KEY=VALUE, repeatable; omit entirely to clear every var")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsEnvironmentsEnvSetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps environments env-set", "environment id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	updated, err := client.SetEnvironmentEnv(context.Background(), id, vars)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set env vars for environment %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "environment %q env vars replaced (%d set)\n", id, len(updated))
	})
}

func appsEnvironmentsEnvSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]

Replaces an environment's entire set of shared env vars in one call,
the same full-replace semantics PUT /apps/{name}'s own env field has.
Every key not passed via --var is removed; running with no --var flags
clears every var.

Flags:
  --var KEY=VALUE          shared env var, repeatable; omit entirely to clear every var
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the resulting env vars as a JSON object to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
