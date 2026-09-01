package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// stringMapFlag is a flag.Value for a repeatable "KEY=VALUE" flag (e.g.
// --build-arg), accumulating into a map. Registered via fs.Var: the
// stdlib flag package has no native repeated-string-flag support.
type stringMapFlag map[string]string

func (m stringMapFlag) String() string {
	if len(m) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, k+"="+v)
	}
	return strings.Join(pairs, ",")
}

// Set splits s on the first "=" and records it. Called once per
// occurrence of the flag on the command line.
func (m stringMapFlag) Set(s string) error {
	key, value, ok := strings.Cut(s, "=")
	if !ok || key == "" {
		return fmt.Errorf("invalid KEY=VALUE %q, want a key, an \"=\", and a value", s)
	}
	m[key] = value
	return nil
}

// apiFlagSet builds a FlagSet named prog+" "+cmdLabel with the
// --token/--api-url/--profile/--json flags most subcommands take, so
// each command only wires up the flags unique to itself.
func apiFlagSet(prog, cmdLabel, jsonUsage string, stderr io.Writer) (fs *flag.FlagSet, tokenFlag, apiURLFlag, profileFlag *string, jsonOut *bool) {
	fs = flag.NewFlagSet(prog+" "+cmdLabel, flag.ContinueOnError)
	fs.SetOutput(stderr)
	tokenFlag = fs.String("token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	apiURLFlag = fs.String("api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	profileFlag = fs.String("profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	jsonOut = fs.Bool("json", false, jsonUsage)
	return fs, tokenFlag, apiURLFlag, profileFlag, jsonOut
}

// apiClientFromFlags builds the API client from resolved
// --token/--api-url/--profile flag values, the
// NewClient(resolveAPIURL(...), resolveToken(...)) call most
// subcommands make.
func apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag string, lookupEnv func(string) (string, bool)) *Client {
	profile := resolveProfile(profileFlag, lookupEnv)
	return NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))
}

// credentialFlags bundles the raw, still-unresolved --token/--api-url/
// --profile flag values a caller passes down together, keeping
// functions like runAppsCreateWizard and runWizardCreateViaAPI under
// golangci-lint's parameter-count limit the same way apiFlagPtrs does
// for their pre-parse pointer counterparts.
type credentialFlags struct {
	Token, APIURL, Profile string
}

// apiClientFromCredentialFlags is apiClientFromFlags taking cf as one
// value instead of three separate strings.
func apiClientFromCredentialFlags(prog string, cf credentialFlags, lookupEnv func(string) (string, bool)) *Client {
	return apiClientFromFlags(prog, cf.APIURL, cf.Token, cf.Profile, lookupEnv)
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

// apiFlagPtrs bundles apiFlagSet's four pointer outputs (still
// unresolved: read only after fs.Parse succeeds) so functions like
// parseSingleArgClient and parseEnvironmentIDCommand stay under
// golangci-lint's parameter-count limit.
type apiFlagPtrs struct {
	token, apiURL, profile *string
	jsonOut                *bool
}

// parseAPIFlags runs fs.Parse (reordering flags before any positional
// arguments first, a no-op when a command takes none) and, on success,
// resolves flags' four pointers into plain values. ok is false once
// fs.Parse or --help has already written its own message to stderr (or
// produced exitOK for -h); the caller should return exitCode unchanged
// in that case. This is the parse-then-deref sequence every apiFlagSet
// caller needs before its own request logic diverges, shared here
// rather than repeated inline (this exact block was flagged as
// duplicated code across dozens of commands before this function
// existed).
func parseAPIFlags(fs *flag.FlagSet, args []string, flags apiFlagPtrs) (tokenFlag, apiURLFlag, profileFlag string, jsonOut bool, exitCode int, ok bool) {
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return "", "", "", false, exitOK, false
		}
		return "", "", "", false, exitUsage, false
	}
	return *flags.token, *flags.apiURL, *flags.profile, *flags.jsonOut, 0, true
}

// parseSingleArgClient runs the parse-flags-then-require-one-arg-then-
// build-client sequence every "verb <name> [flags]" subcommand needs
// before its own request logic diverges (apps git-source get/set/delete,
// apps secrets list, the identical shape apps_group.go/apps_log_drain.go
// already established). ok is false once fs.Parse, --help, or the
// missing-arg check has already written its own message to stderr; the
// caller should return exitCode unchanged in that case.
func parseSingleArgClient(fs *flag.FlagSet, args []string, flags apiFlagPtrs, stderr io.Writer, prog, cmdLabel string, lookupEnv func(string) (string, bool)) (client *Client, name string, jsonOut bool, exitCode int, ok bool) {
	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, flags)
	if !ok {
		return nil, "", false, exitCode, false
	}

	name, ok = requireOneArg(fs, stderr, prog, cmdLabel, "app name")
	if !ok {
		return nil, "", false, exitUsage, false
	}

	return apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), name, jsonOut, exitOK, true
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
