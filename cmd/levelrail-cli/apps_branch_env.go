package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsBranchEnv dispatches "apps branch-env <verb> [flags]" to one of
// list/set/clear: GET/POST/DELETE /api/v1/apps/{name}/branch-env(/{id})
// (internal/api/apps_branch_env.go). A branch-scoped sibling of "apps
// preview-env": that command's override applies to every preview
// unconditionally, this one only when the preview's own branch matches
// a declared pattern.
func runAppsBranchEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBranchEnvUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBranchEnvUsage(prog))
		return exitOK
	case "list":
		return runAppsBranchEnvList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsBranchEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsBranchEnvClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps branch-env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsBranchEnvUsage(prog))
		return exitUsage
	}
}

func appsBranchEnvUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps branch-env list <name> [flags]                                                   list an app's branch-scoped env var overrides
  %[1]s apps branch-env set <name> <key> --branch PATTERN --value VALUE [--secret] [flags]     declare (or replace) a branch-scoped env var override
  %[1]s apps branch-env clear <name> <id> [flags]                                              remove one override by its id (from "list" or "set")

Declares one env var to take a different value only when a preview
environment is created from a branch matching PATTERN (an exact branch
name, or a shell glob like "release/*"), replacing whatever that key
would otherwise inherit: this app's own env, or an unscoped "apps
preview-env" override. --secret stores the value envelope-encrypted and
never returns it again, the same as "apps secrets set". Never affects
<name>'s own running deploy.

Run "%[1]s apps branch-env <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsBranchEnvList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps branch-env list", "print the overrides as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps branch-env list <name> [flags]\n\nLists an app's branch-scoped env var overrides. A secret-marked entry's\nvalue is always empty.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps branch-env list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	overrides, err := client.ListAppBranchEnv(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list branch env overrides for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, overrides, func() { printBranchEnvOverridesHuman(stdout, overrides) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printBranchEnvOverridesHuman(stdout io.Writer, overrides []apiclient.AppBranchEnvOverride) {
	if len(overrides) == 0 {
		_, _ = fmt.Fprintln(stdout, "no branch env overrides")
		return
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tBRANCH\tKEY\tVALUE\tSECRET")
	for _, o := range overrides {
		value := o.Value
		if o.Secret {
			value = "(hidden)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\n", o.ID, o.BranchPattern, o.Key, value, o.Secret)
	}
	_ = tw.Flush()
}

func runAppsBranchEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps branch-env set", "print the resulting override as JSON to stdout and nothing else", stderr)
	var branch, value string
	var secret bool
	fs.StringVar(&branch, "branch", "", "exact branch name, or a shell glob like \"release/*\" (required)")
	fs.StringVar(&value, "value", "", "the branch-scoped value for this env var (required)")
	fs.BoolVar(&secret, "secret", false, "store this value envelope-encrypted; it is never returned again")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps branch-env set <name> <key> --branch PATTERN --value VALUE [--secret] [flags]\n\nDeclares <key>'s branch-scoped value on app <name>. Setting the same\n--branch and <key> again replaces the existing override in place.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	cmd := twoArgCmd{prog: prog, cmdLabel: "apps branch-env set", argsLabel: "an app name and an env var key"}
	client, name, key, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, cmd, lookupEnv)
	if !ok {
		return exitCode
	}
	if branch == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps branch-env set requires --branch\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if value == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps branch-env set requires --value\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	result, err := client.SetAppBranchEnv(context.Background(), name, branch, key, value, secret)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set branch env override %q for app %q: %w", key, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		if result.Secret {
			_, _ = fmt.Fprintf(stdout, "%s: id=%s branch=%s key=%s secret=true\n", name, result.ID, result.BranchPattern, result.Key)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s: id=%s branch=%s key=%s value=%s\n", name, result.ID, result.BranchPattern, result.Key, result.Value)
	})
}

func runAppsBranchEnvClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps branch-env clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps branch-env clear <name> <id> [flags]\n\nRemoves one branch-scoped override from app <name> by its id, as shown\nby \"apps branch-env list\" or \"apps branch-env set\".\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	cmd := twoArgCmd{prog: prog, cmdLabel: "apps branch-env clear", argsLabel: "an app name and an override id"}
	client, name, id, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, cmd, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.DeleteAppBranchEnv(context.Background(), name, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear branch env override %q for app %q: %w", id, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "branch env override %q cleared for app %q\n", id, name)
	})
}
