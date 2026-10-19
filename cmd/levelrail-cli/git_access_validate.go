package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runAppsGitSourceValidate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source validate", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source validate <name> [flags]\n\nReads the app's branch with the credential a deploy would use and says what to fix.\nExits non-zero when access fails.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source validate", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.ValidateGitSourceAccess(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("validate git access for app %q: %w", name, err))
	}
	return finishGitAccess(stdout, stderr, of, res)
}

func runGitProvidersValidate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "git-providers validate", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s git-providers validate <github|gitlab|gitea|bitbucket> [flags]\n\nChecks that the provider connection holds a working credential and says what to fix.\nExits non-zero when it does not.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, provider, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "git-providers validate", "provider"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.ValidateGitProvider(context.Background(), provider)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("validate git provider %q: %w", provider, err))
	}
	return finishGitAccess(stdout, stderr, of, res)
}

func finishGitAccess(stdout, stderr io.Writer, of outputFlags, res apiclient.GitAccessResult) int {
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		for _, c := range res.Checks {
			_, _ = fmt.Fprintf(stdout, "%-12s %-8s %s\n", c.ID, c.Status, c.Detail)
		}
		if res.Fix != nil {
			_, _ = fmt.Fprintf(stdout, "fix (%s): %s\n", res.Fix.Kind, res.Fix.Hint)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !res.OK {
		return exitCheckFailed
	}
	return exitOK
}
