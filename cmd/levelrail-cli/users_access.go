package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runUsersRole dispatches "users role set <user> <role>" (experimental access-roles).
func runUsersRole(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "set" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s users role set <user> <role> [flags]\n", prog)
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "users role set", "print the updated user as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s users role set <user> <role> [flags]\n\nAssigns a stored role (id or name) and copies its abilities onto the user.\nRefused for your own user and when it would remove the last root user.\n"+rolesFlagsHelp, prog, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "users role set", "a user id and a role", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	role, err := findRole(ctx, client, rest[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	updated, err := client.SetUserRole(ctx, rest[0], role.ID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set role for user %q: %w", rest[0], err))
	}
	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stdout, "user %q is now %q\n", updated.Email, role.Name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runUsersGrants dispatches "users grants get|set <user>" (experimental access-roles).
func runUsersGrants(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s users grants get <user> [flags]\n  %[1]s users grants set <user> [--environment ID ...] [flags]\n", prog)
		return exitUsage
	}
	verb := args[0]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "users grants "+verb, "print the grants as JSON to stdout and nothing else", stderr)
	var envs stringList
	if verb == "set" {
		fs.Var(&envs, "environment", "environment id to grant (repeatable; none clears every grant)")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s users grants get <user> [flags]\n  %[1]s users grants set <user> [--environment ID ...] [flags]\n\nGrants list the environments a user whose role has granted visibility (guest) can see.\nset replaces the whole list.\n"+rolesFlagsHelp, prog, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	userID, ok := requireOneArg(fs, stderr, prog, "users grants "+verb, "user id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var grants apiclient.EnvironmentGrants
	var err error
	if verb == "get" {
		grants, err = client.GetUserEnvironmentGrants(context.Background(), userID)
	} else {
		grants, err = client.SetUserEnvironmentGrants(context.Background(), userID, envs)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s environment grants for user %q: %w", verb, userID, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, grants, func() {
		if len(grants.EnvironmentIDs) == 0 {
			_, _ = fmt.Fprintln(stdout, "no environments granted")
			return
		}
		_, _ = fmt.Fprintln(stdout, strings.Join(grants.EnvironmentIDs, "\n"))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
