package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
)

// runGitLabApp dispatches "gitlab-app <verb> [flags]" to one of
// projects/branches/use-as-source: internal/api/gitlab_app_projects.go's
// three routes, the GitLab counterpart of runGitHubApp. Connecting the
// GitLab OAuth Application itself stays dashboard-only, same reasoning.
// Unlike github-app/bitbucket-app's owner+repo pair, GitLab addresses a
// project by a single numeric ID, so branches/use-as-source stay their
// own implementations here rather than going through
// runTwoArgList/runTwoArgUseAsSource (list_command.go).
func runGitLabApp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, gitlabAppUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, gitlabAppUsage(prog))
		return exitOK
	case "projects":
		return runGitLabAppProjects(prog, args[1:], stdout, stderr, lookupEnv)
	case "branches":
		return runGitLabAppBranches(prog, args[1:], stdout, stderr, lookupEnv)
	case "use-as-source":
		return runGitLabAppUseAsSource(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown gitlab-app subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, gitlabAppUsage(prog))
		return exitUsage
	}
}

func gitlabAppUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s gitlab-app projects [flags]                                     list projects the connected account can access
  %[1]s gitlab-app branches <project-id> [flags]                        list a project's branches
  %[1]s gitlab-app use-as-source <project-id> --app-name NAME [flags]   connect a project as an app's git source

Connecting the GitLab App itself is dashboard-only (a real browser
redirect through GitLab's OAuth2 authorization endpoint); once connected,
these subcommands browse and use its projects from the CLI.

Run "%[1]s gitlab-app <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runGitLabAppProjects(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[[]gitLabAppProjectResource]{
		cmdLabel:  "gitlab-app projects",
		jsonUsage: "print projects as a JSON array to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s gitlab-app projects [flags]\n\nLists every project the connected GitLab account can access.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) ([]gitLabAppProjectResource, error) {
			return c.ListGitLabAppProjects(ctx)
		},
		errVerb: "list gitlab app projects",
		print:   printGitLabAppProjectsTable,
	})
}

func printGitLabAppProjectsTable(out io.Writer, projects []gitLabAppProjectResource) {
	if len(projects) == 0 {
		_, _ = fmt.Fprintln(out, "no projects")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tPATH_WITH_NAMESPACE\tVISIBILITY\tDEFAULT_BRANCH")
	for _, p := range projects {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", p.ID, p.PathWithNamespace, p.Visibility, p.DefaultBranch)
	}
	_ = tw.Flush()
}

// parseGitLabProjectID parses <project-id>, reporting a usage error
// through fs.Usage (matching gitlab_app_projects.go's own "id must be a
// positive integer" validation) rather than a bare strconv error.
func parseGitLabProjectID(prog, cmdLabel, raw string, stderr io.Writer, usage func()) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		_, _ = fmt.Fprintf(stderr, "%s: %s: project-id must be a positive integer\n\n", prog, cmdLabel)
		usage()
		return 0, false
	}
	return id, true
}

func runGitLabAppBranches(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "gitlab-app branches", "print branches as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s gitlab-app branches <project-id> [flags]\n\nLists a project's branches.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rawID, ok := requireOneArg(fs, stderr, prog, "gitlab-app branches", "project id")
	if !ok {
		return exitUsage
	}
	projectID, ok := parseGitLabProjectID(prog, "gitlab-app branches", rawID, stderr, fs.Usage)
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	branches, err := client.ListGitLabAppBranches(context.Background(), projectID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list branches for project %d: %w", projectID, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, branches, func() { printGitAppBranchesTable(stdout, branches) })
}

func runGitLabAppUseAsSource(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "gitlab-app use-as-source", "print the resulting git source as JSON to stdout and nothing else", stderr)
	req := bindUseAsSourceFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s gitlab-app use-as-source <project-id> --app-name NAME [flags]\n\nConnects the project as NAME's git source and registers a push webhook.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rawID, ok := requireOneArg(fs, stderr, prog, "gitlab-app use-as-source", "project id")
	if !ok {
		return exitUsage
	}
	projectID, ok := parseGitLabProjectID(prog, "gitlab-app use-as-source", rawID, stderr, fs.Usage)
	if !ok {
		return exitUsage
	}
	if req.AppName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: gitlab-app use-as-source requires --app-name\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.UseGitLabProjectAsSource(context.Background(), projectID, *req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("use project %d as source for app %q: %w", projectID, req.AppName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printGitSourceHuman(stdout, result) })
}
