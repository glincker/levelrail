package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runBitbucketApp dispatches "bitbucket-app <verb> [flags]" to one of
// repos/branches/use-as-source: internal/api/bitbucket_app_repos.go's
// three routes, the Bitbucket counterpart of runGitHubApp/runGitLabApp.
// Connecting the Bitbucket OAuth consumer itself stays dashboard-only,
// same reasoning as the other two providers.
func runBitbucketApp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, bitbucketAppUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, bitbucketAppUsage(prog))
		return exitOK
	case "repos":
		return runBitbucketAppRepos(prog, args[1:], stdout, stderr, lookupEnv)
	case "branches":
		return runBitbucketAppBranches(prog, args[1:], stdout, stderr, lookupEnv)
	case "use-as-source":
		return runBitbucketAppUseAsSource(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown bitbucket-app subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, bitbucketAppUsage(prog))
		return exitUsage
	}
}

func bitbucketAppUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s bitbucket-app repos [flags]                                                list repos the connected account can access
  %[1]s bitbucket-app branches <workspace> <repo-slug> [flags]                     list a repo's branches
  %[1]s bitbucket-app use-as-source <workspace> <repo-slug> --app-name NAME [flags]   connect a repo as an app's git source

Connecting the Bitbucket App itself is dashboard-only (a real browser
redirect through Bitbucket's OAuth consumer flow); once connected, these
subcommands browse and use its repos from the CLI.

Run "%[1]s bitbucket-app <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runBitbucketAppRepos(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bitbucket-app repos", "print repos as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s bitbucket-app repos [flags]\n\nLists every repository the connected Bitbucket account can access.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	repos, err := client.ListBitbucketAppRepos(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list bitbucket app repos: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, repos, func() { printBitbucketAppReposTable(stdout, repos) })
}

func printBitbucketAppReposTable(out io.Writer, repos []bitbucketAppRepoResource) {
	if len(repos) == 0 {
		_, _ = fmt.Fprintln(out, "no repositories")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FULL_NAME\tPRIVATE\tDEFAULT_BRANCH")
	for _, r := range repos {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\n", r.FullName, r.Private, r.DefaultBranch)
	}
	_ = tw.Flush()
}

func runBitbucketAppBranches(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bitbucket-app branches", "print branches as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s bitbucket-app branches <workspace> <repo-slug> [flags]\n\nLists a repo's branches.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "bitbucket-app branches", "a workspace and a repo slug", 2)
	if !ok {
		return exitUsage
	}
	workspace, repoSlug := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	branches, err := client.ListBitbucketAppBranches(context.Background(), workspace, repoSlug)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list branches for %s/%s: %w", workspace, repoSlug, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, branches, func() { printGitAppBranchesTable(stdout, branches) })
}

func runBitbucketAppUseAsSource(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bitbucket-app use-as-source", "print the resulting git source as JSON to stdout and nothing else", stderr)
	req := bindUseAsSourceFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s bitbucket-app use-as-source <workspace> <repo-slug> --app-name NAME [flags]\n\nConnects the repo as NAME's git source and registers a push webhook.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "bitbucket-app use-as-source", "a workspace and a repo slug", 2)
	if !ok {
		return exitUsage
	}
	workspace, repoSlug := rest[0], rest[1]
	if req.AppName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: bitbucket-app use-as-source requires --app-name\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.UseBitbucketRepoAsSource(context.Background(), workspace, repoSlug, *req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("use %s/%s as source for app %q: %w", workspace, repoSlug, req.AppName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printGitSourceHuman(stdout, result) })
}
