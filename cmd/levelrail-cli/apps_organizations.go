package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
)

// runAppsOrganizations dispatches "apps organizations <verb> [flags]" to
// one of create/list/get/delete/set-project/clear-project, the CLI
// counterpart of internal/api/organizations.go's own routes. An
// organization groups projects (internal/api/organizations.go's own doc
// comment); it has no direct app or database membership of its own.
func runAppsOrganizations(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsOrganizationsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsOrganizationsUsage(prog))
		return exitOK
	case "create":
		return runAppsOrganizationsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runAppsOrganizationsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runAppsOrganizationsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsOrganizationsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "set-project":
		return runAppsOrganizationsSetProject(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear-project":
		return runAppsOrganizationsClearProject(prog, args[1:], stdout, stderr, lookupEnv)
	case "env-get":
		return runAppsOrganizationsEnvGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "env-set":
		return runAppsOrganizationsEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps organizations subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsOrganizationsUsage(prog))
		return exitUsage
	}
}

func appsOrganizationsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations create --name NAME [flags]                        create an organization
  %[1]s apps organizations list [flags]                                        list organizations
  %[1]s apps organizations get <id> [flags]                                    show one organization
  %[1]s apps organizations delete <id> [flags]                                 delete an organization
  %[1]s apps organizations set-project <project-id> <org-id> [flags]        file a project under an organization
  %[1]s apps organizations clear-project <project-id> [flags]                 remove a project from its organization
  %[1]s apps organizations env-get <id> [flags]                                show an organization's shared env vars
  %[1]s apps organizations env-set <id> --var KEY=VALUE [flags]           replace an organization's shared env vars

An organization groups projects; deleting one leaves its member projects
running, simply organization-less again. Its shared env vars are the base
layer every member project's own shared env vars override.

Run "%[1]s apps organizations <subcommand> -h" for a subcommand's own
flags.
`, prog)
}

func runAppsOrganizationsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations create", "print the created organization as JSON to stdout and nothing else", stderr)
	var name string
	fs.StringVar(&name, "name", "", "organization name (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateOrganization(context.Background(), createOrganizationRequest{Name: name})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create organization %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, created, func() {
		_, _ = fmt.Fprintf(stdout, "organization %q created (id %s)\n", created.Name, created.ID)
	})
}

func appsOrganizationsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations create --name NAME [flags]

Creates a new organization.

Flags:
  --name string            organization name (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the created organization as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations list", "print organizations as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	orgs, err := client.ListOrganizations(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list organizations: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, orgs, func() { printOrganizationsTable(stdout, orgs) })
}

func printOrganizationsTable(out io.Writer, orgs []organizationResource) {
	if len(orgs) == 0 {
		_, _ = fmt.Fprintln(out, "no organizations")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCREATED")
	for _, o := range orgs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", o.ID, o.Name, o.CreatedAt)
	}
	_ = tw.Flush()
}

func appsOrganizationsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations list [flags]

Lists every organization.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print organizations as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations get", "print the organization as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsGetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "apps organizations get", "organization id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	org, err := client.GetOrganization(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get organization %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, org, func() {
		_, _ = fmt.Fprintf(stdout, "id:      %s\nname:    %s\ncreated: %s\n", org.ID, org.Name, org.CreatedAt)
	})
}

func appsOrganizationsGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations get <id> [flags]

Shows one organization.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the organization as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations delete", "print {} to stdout on success instead of a plain confirmation", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "apps organizations delete", "organization id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteOrganization(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete organization %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, map[string]any{}, func() {
		_, _ = fmt.Fprintf(stdout, "organization %q deleted\n", id)
	})
}

func appsOrganizationsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations delete <id> [flags]

Deletes an organization. Every project filed under it survives,
simply organization-less again.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {} to stdout on success instead of a plain confirmation
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsSetProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations set-project", "print the updated project as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsSetProjectUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps organizations set-project", "a project id and an organization id", 2)
	if !ok {
		return exitUsage
	}
	projectID, orgID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetProjectOrganization(context.Background(), projectID, orgID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set organization for project %q: %w", projectID, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "project %q filed under organization %q\n", projectID, orgID)
	})
}

func appsOrganizationsSetProjectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations set-project <project-id> <org-id> [flags]

Files an existing project under an organization.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated project as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsClearProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations clear-project", "print the updated project as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsClearProjectUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	projectID, ok := requireOneArg(fs, stderr, prog, "apps organizations clear-project", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetProjectOrganization(context.Background(), projectID, "")
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear organization for project %q: %w", projectID, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "project %q removed from its organization\n", projectID)
	})
}

func appsOrganizationsClearProjectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations clear-project <project-id> [flags]

Removes a project from its organization, leaving it organization-less.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated project as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsEnvGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations env-get", "print the env vars as a JSON object to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsEnvGetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "apps organizations env-get", "organization id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	vars, err := client.GetOrganizationEnv(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get env vars for organization %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, vars, func() { printEnvVarsTable(stdout, vars) })
}

func printEnvVarsTable(out io.Writer, vars map[string]string) {
	if len(vars) == 0 {
		_, _ = fmt.Fprintln(out, "no env vars set")
		return
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE")
	for _, k := range keys {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", k, vars[k])
	}
	_ = tw.Flush()
}

func appsOrganizationsEnvGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations env-get <id> [flags]

Shows an organization's shared env vars: the base layer every member
project's own shared env vars, and every project's apps, override.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the env vars as a JSON object to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps organizations env-set", "print the resulting env vars as a JSON object to stdout and nothing else", stderr)
	vars := stringMapFlag{}
	fs.Var(vars, "var", "shared env var as KEY=VALUE, repeatable; omit entirely to clear every var")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsEnvSetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "apps organizations env-set", "organization id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetOrganizationEnv(context.Background(), id, vars)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set env vars for organization %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "organization %q env vars replaced (%d set)\n", id, len(updated))
	})
}

func appsOrganizationsEnvSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps organizations env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]

Replaces an organization's entire set of shared env vars in one call,
the same full-replace semantics PUT /apps/{name}'s own env field has.
Every key not passed via --var is removed; running with no --var flags
at all clears every var.

Flags:
  --var KEY=VALUE         shared env var, repeatable
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the resulting env vars as a JSON object to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
