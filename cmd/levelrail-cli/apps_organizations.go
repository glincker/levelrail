package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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

An organization groups projects; deleting one leaves its member projects
running, simply organization-less again.

Run "%[1]s apps organizations <subcommand> -h" for a subcommand's own
flags.
`, prog)
}

func runAppsOrganizationsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps organizations create", "print the created organization as JSON to stdout and nothing else", stderr)
	var name string
	fs.StringVar(&name, "name", "", "organization name (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
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
  --json                     print the created organization as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps organizations list", "print organizations as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
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
  --json                    print organizations as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps organizations get", "print the organization as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsGetUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps organizations get", "organization id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
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
  --json                    print the organization as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps organizations delete", "print {} to stdout on success instead of a plain confirmation", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "apps organizations delete", "organization id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
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
  --json                    print {} to stdout on success instead of a plain confirmation
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsSetProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps organizations set-project", "print the updated project as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsSetProjectUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	rest, ok := requireArgs(fs, stderr, prog, "apps organizations set-project", "a project id and an organization id", 2)
	if !ok {
		return exitUsage
	}
	projectID, orgID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
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
  --json                    print the updated project as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsOrganizationsClearProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps organizations clear-project", "print the updated project as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsOrganizationsClearProjectUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	projectID, ok := requireOneArg(fs, stderr, prog, "apps organizations clear-project", "project id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
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
  --json                    print the updated project as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
