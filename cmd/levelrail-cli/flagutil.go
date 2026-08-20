package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// apiFlagSet builds a FlagSet named prog+" "+cmdLabel with the
// --token/--api-url/--json flags most subcommands take, so each command
// only wires up the flags unique to itself.
func apiFlagSet(prog, cmdLabel, jsonUsage string, stderr io.Writer) (fs *flag.FlagSet, tokenFlag, apiURLFlag *string, jsonOut *bool) {
	fs = flag.NewFlagSet(prog+" "+cmdLabel, flag.ContinueOnError)
	fs.SetOutput(stderr)
	tokenFlag = fs.String("token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	apiURLFlag = fs.String("api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	jsonOut = fs.Bool("json", false, jsonUsage)
	return fs, tokenFlag, apiURLFlag, jsonOut
}

// apiClientFromFlags builds the API client from resolved --token/--api-url
// flag values, the NewClient(resolveAPIURL(...), resolveToken(...)) call
// most subcommands make.
func apiClientFromFlags(prog, apiURLFlag, tokenFlag string, lookupEnv func(string) (string, bool)) *Client {
	return NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
}

// requireArgs extracts fs's exactly n required positional arguments,
// labeled argsLabel in the usage error a subcommand like "apps
// scheduled-tasks delete <app> <id>" prints when the count doesn't
// match (e.g. "an app name and a task id").
func requireArgs(fs *flag.FlagSet, stderr io.Writer, prog, cmdLabel, argsLabel string, n int) ([]string, bool) {
	rest := fs.Args()
	if len(rest) != n {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires %s\n\n", prog, cmdLabel, argsLabel)
		fs.Usage()
		return nil, false
	}
	return rest, true
}

// requireOneArg extracts fs's single required positional argument,
// labeled argLabel in the usage error a subcommand like "channels
// delete <id>" or "apps log-drain get <name>" prints when it's missing.
func requireOneArg(fs *flag.FlagSet, stderr io.Writer, prog, cmdLabel, argLabel string) (string, bool) {
	rest, ok := requireArgs(fs, stderr, prog, cmdLabel, "exactly one "+argLabel, 1)
	if !ok {
		return "", false
	}
	return rest[0], true
}

// reorderArgsFlagsFirst rewrites args so every flag (and its value, if
// it takes one) is moved before any positional argument, then hands
// back a slice fs.Parse can consume normally.
//
// This works around a real stdlib flag.FlagSet limitation: Parse stops
// consuming flags at the first non-flag token, so a natural, common
// invocation shape like "apps get web --json" (name first, flags after,
// the same order `docker inspect <id> --format` or `git show <sha>
// --stat` accept) would otherwise leave --json sitting unparsed in
// fs.Args() instead of setting the flag, silently producing the wrong
// behavior rather than an error. fs must already have every flag it
// will accept defined (via StringVar/BoolVar/etc.) before this is
// called, since that's how this function tells a boolean flag (no
// value token follows it) from a value flag (the next token belongs to
// it, not to the positional arguments).
func reorderArgsFlagsFirst(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) == 0 || a[0] != '-' || a == "-" {
			positional = append(positional, a)
			continue
		}

		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			// "--flag=value" already carries its value in this one
			// token, nothing more to consume.
			continue
		}
		fl := fs.Lookup(name)
		if fl == nil {
			// Unknown flag: leave it to fs.Parse to report the real
			// error rather than guessing whether it takes a value.
			continue
		}
		if bv, ok := fl.Value.(interface{ IsBoolFlag() bool }); ok && bv.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}
