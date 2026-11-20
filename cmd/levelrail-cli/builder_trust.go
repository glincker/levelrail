package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// runNodesBuildTrust implements "nodes build-trust <node> <server|managed|personal>".
func runNodesBuildTrust(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes build-trust", "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesBuildTrustUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "nodes build-trust", "a node id and a tier", 2)
	if !ok {
		return exitUsage
	}
	id, tier := rest[0], rest[1]
	if tier != "server" && tier != "managed" && tier != "personal" {
		return reportError(stdout, stderr, jsonOut, newValidationError("tier must be server, managed or personal"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetNodeBuildTrust(context.Background(), id, tier)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set build trust for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "node %q build trust: %s\n", res.ID, res.BuildTrust)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesBuildTrustUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes build-trust <node> <server|managed|personal> [flags]

Sets the builder tier of a node. Personal builders (laptops, home machines)
are not used for production environments unless an environment allows them.
`, prog)
}

// runEnvironmentsBuilders implements "environments builders get|set <id> [tier|node ...]".
func runEnvironmentsBuilders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, environmentsBuildersUsage(prog))
		return exitUsage
	}
	action := args[0]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "environments builders "+action, "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, environmentsBuildersUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) == 0 || (action == "get" && len(rest) != 1) {
		_, _ = fmt.Fprintf(stderr, "%s: environments builders %s requires an environment id\n\n", prog, action)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	res, err := client.GetEnvironmentBuilders(ctx, rest[0])
	if err == nil && action == "set" {
		res, err = client.SetEnvironmentBuilders(ctx, rest[0], rest[1:])
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("builders of environment %q: %w", rest[0], err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		allowed := "default policy"
		if len(res.AllowedBuilders) > 0 {
			allowed = strings.Join(res.AllowedBuilders, ", ")
		}
		_, _ = fmt.Fprintf(stdout, "environment %s (%s): allowed builders: %s; personal builders allowed: %t\n", res.EnvironmentID, res.Kind, allowed, res.PersonalAllowed)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func environmentsBuildersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s environments builders get <id> [flags]
  %[1]s environments builders set <id> [server|managed|personal|<build node> ...] [flags]

Shows or replaces the builders an environment accepts. Set with no entries
to restore the default policy (personal builders excluded from production).
`, prog)
}
