package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsProjects dispatches "apps projects <verb> [flags]" to one of
// create/list/get/delete/env-get/env-set, the CLI counterpart of
// internal/api/projects.go's own routes. A project groups apps and
// databases (internal/api/projects.go's own doc comment); there is no
// update route, since a project's name isn't an addressing key and every
// other field (org_id) has its own dedicated route (see "apps
// organizations set-project"/"clear-project").
func runAppsProjects(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsProjectsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsProjectsUsage(prog))
		return exitOK
	case "create":
		return runAppsProjectsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runAppsProjectsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runAppsProjectsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsProjectsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "env-get":
		return runAppsProjectsEnvGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "env-set":
		return runAppsProjectsEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps projects subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsProjectsUsage(prog))
		return exitUsage
	}
}

func appsProjectsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects create --name NAME [flags]        create a project
  %[1]s apps projects list [flags]                         list projects
  %[1]s apps projects get <id> [flags]                     show one project
  %[1]s apps projects delete <id> [flags]                  delete a project
  %[1]s apps projects env-get <id> [flags]                 show a project's shared env vars
  %[1]s apps projects env-set <id> --var KEY=VALUE [flags]   replace a project's shared env vars

A project groups apps and databases (move one in with "%[1]s apps
set-project" or "%[1]s databases set-project"); deleting a project
leaves its members running, simply project-less again. File a project
under an organization with "%[1]s apps organizations set-project". Its
shared env vars sit between its organization's own shared env vars and
any of its environments' shared env vars.

Run "%[1]s apps projects <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsProjectsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps projects create", "print the created project as JSON to stdout and nothing else", stderr)
	var name string
	fs.StringVar(&name, "name", "", "project name (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateProject(context.Background(), createProjectRequest{Name: name})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create project %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, created, func() {
		_, _ = fmt.Fprintf(stdout, "project %q created (id %s)\n", created.Name, created.ID)
	})
}

func appsProjectsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects create --name NAME [flags]

Creates a new project.

Flags:
  --name string            project name (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the created project as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsProjectsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps projects list", "print projects as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	projects, err := client.ListProjects(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list projects: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, projects, func() { printProjectsTable(stdout, projects) })
}

func printProjectsTable(out io.Writer, projects []projectResource) {
	if len(projects) == 0 {
		_, _ = fmt.Fprintln(out, "no projects")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tORG_ID\tCREATED")
	for _, p := range projects {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, p.Name, p.OrgID, p.CreatedAt)
	}
	_ = tw.Flush()
}

func appsProjectsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects list [flags]

Lists every project.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print projects as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsProjectsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps projects get", "print the project as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsGetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps projects get", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	p, err := client.GetProject(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get project %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, p, func() {
		_, _ = fmt.Fprintf(stdout, "id:      %s\nname:    %s\norg_id:  %s\ncreated: %s\n", p.ID, p.Name, p.OrgID, p.CreatedAt)
	})
}

func appsProjectsGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects get <id> [flags]

Shows one project.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the project as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsProjectsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps projects delete", "print {} to stdout on success instead of a plain confirmation", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps projects delete", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteProject(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete project %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, map[string]any{}, func() {
		_, _ = fmt.Fprintf(stdout, "project %q deleted\n", id)
	})
}

func appsProjectsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects delete <id> [flags]

Deletes a project. Every app and database filed under it survives,
simply project-less again.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {} to stdout on success instead of a plain confirmation
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsProjectsEnvGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps projects env-get", "print the env vars as a JSON object to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsEnvGetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps projects env-get", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	vars, err := client.GetProjectEnv(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get env vars for project %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, vars, func() { printEnvVarsTable(stdout, vars) })
}

func appsProjectsEnvGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects env-get <id> [flags]

Shows a project's shared env vars: overrides its organization's own
shared env vars (if filed under one), and is itself overridden by any
of its environments' shared env vars and by a tagged app's own env.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the env vars as a JSON object to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsProjectsEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps projects env-set", "print the resulting env vars as a JSON object to stdout and nothing else", stderr)
	vars := stringMapFlag{}
	fs.Var(vars, "var", "shared env var as KEY=VALUE, repeatable; omit entirely to clear every var")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProjectsEnvSetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps projects env-set", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetProjectEnv(context.Background(), id, vars)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set env vars for project %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "project %q env vars replaced (%d set)\n", id, len(updated))
	})
}

func appsProjectsEnvSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps projects env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]

Replaces a project's entire set of shared env vars in one call, the
same full-replace semantics PUT /apps/{name}'s own env field has. Every
key not passed via --var is removed; running with no --var flags at
all clears every var.

Flags:
  --var KEY=VALUE         shared env var, repeatable
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the resulting env vars as a JSON object to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
