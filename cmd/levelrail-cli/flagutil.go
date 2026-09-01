package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
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
// --token/--api-url/--profile/--json/--output/--query flags most
// subcommands take, so each command only wires up the flags unique to
// itself. --json is kept as shorthand for --output json (see
// resolveOutputFormat) so existing scripts built against it keep
// working unchanged.
func apiFlagSet(prog, cmdLabel, jsonUsage string, stderr io.Writer) (fs *flag.FlagSet, tokenFlag, apiURLFlag, profileFlag *string, jsonOut *bool, outputFlag, queryFlag *string) {
	fs = flag.NewFlagSet(prog+" "+cmdLabel, flag.ContinueOnError)
	fs.SetOutput(stderr)
	tokenFlag = fs.String("token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	apiURLFlag = fs.String("api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	profileFlag = fs.String("profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	jsonOut = fs.Bool("json", false, jsonUsage)
	outputFlag, queryFlag = bindOutputQueryFlags(fs)
	return fs, tokenFlag, apiURLFlag, profileFlag, jsonOut, outputFlag, queryFlag
}

// bindOutputQueryFlags registers --output and --query on fs. Split out
// of apiFlagSet so the handful of commands that build their own
// FlagSet by hand (rather than through apiFlagSet/sessionFlagSet) can
// still get both flags with one call instead of duplicating the flag
// descriptions.
func bindOutputQueryFlags(fs *flag.FlagSet) (outputFlag, queryFlag *string) {
	outputFlag = fs.String("output", "", "output format: json, table, or text (default table; --json is shorthand for --output json)")
	queryFlag = fs.String("query", "", "JMESPath expression to filter the result before printing (e.g. \"[?status=='running'].name\")")
	return outputFlag, queryFlag
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

// sessionFlagSet builds a FlagSet named prog+" "+cmdLabel with the
// --username/--password/--api-url/--profile/--json/--output/--query
// flags every session-authenticated command (tokens create/list/revoke,
// all session-only per tokens.go's own doc comment, never bearer-token
// based) needs, the session-auth counterpart of apiFlagSet.
func sessionFlagSet(prog, cmdLabel, jsonUsage string, stderr io.Writer) (fs *flag.FlagSet, username, password, apiURLFlag, profileFlag *string, jsonOut *bool, outputFlag, queryFlag *string) {
	fs = flag.NewFlagSet(prog+" "+cmdLabel, flag.ContinueOnError)
	fs.SetOutput(stderr)
	username = fs.String("username", "", "admin username (prompted if omitted)")
	password = fs.String("password", "", "admin password (prompted without echo if omitted)")
	apiURLFlag = fs.String("api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	profileFlag = fs.String("profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	jsonOut = fs.Bool("json", false, jsonUsage)
	outputFlag, queryFlag = bindOutputQueryFlags(fs)
	return fs, username, password, apiURLFlag, profileFlag, jsonOut, outputFlag, queryFlag
}

// sessionFlags bundles sessionFlagSet's resolved --username/--password/
// --api-url/--profile flag values, keeping loggedInSessionClient under
// golangci-lint's parameter-count limit.
type sessionFlags struct {
	username, password, apiURLFlag, profileFlag string
}

// loggedInSessionClient resolves username/password (prompting if either
// was omitted, see resolveLoginCredentials), builds a session client
// against sf's resolved API URL, and logs in. Every session-
// authenticated command needs exactly this sequence before its own
// request logic diverges. Returns the server's own login response
// (which callers like "auth login" print the username from) alongside
// the now-authenticated client.
func loggedInSessionClient(ctx context.Context, sf sessionFlags, prog string, lookupEnv func(string) (string, bool), stdin io.Reader, stderr io.Writer) (*authSessionClient, loginResponse, error) {
	readPassword := func() (string, error) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(b), err
	}
	resolvedUsername, resolvedPassword, err := resolveLoginCredentials(sf.username, sf.password, stdin, stderr, readPassword)
	if err != nil {
		return nil, loginResponse{}, err
	}

	profile := resolveProfile(sf.profileFlag, lookupEnv)
	sessionClient, err := newAuthSessionClient(resolveAPIURL(sf.apiURLFlag, lookupEnv, prog, profile))
	if err != nil {
		return nil, loginResponse{}, err
	}

	loginResp, err := sessionClient.Login(ctx, resolvedUsername, resolvedPassword)
	if err != nil {
		return nil, loginResponse{}, fmt.Errorf("log in as %q: %w", resolvedUsername, err)
	}
	return sessionClient, loginResp, nil
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

// apiFlagPtrs bundles apiFlagSet's six pointer outputs (still
// unresolved: read only after fs.Parse succeeds) so functions like
// parseSingleArgClient and parseEnvironmentIDCommand stay under
// golangci-lint's parameter-count limit.
type apiFlagPtrs struct {
	token, apiURL, profile *string
	jsonOut                *bool
	output, query          *string
}

// outputFlags bundles a command's resolved --output/--query values
// (Format already folds in the old --json boolean, see
// resolveOutputFormat), keeping functions like runAppsCreateWizard and
// writeScheduledTaskResult under golangci-lint's parameter-count limit
// the same way credentialFlags does for --token/--api-url/--profile.
type outputFlags struct {
	Format outputFormat
	Query  string
}

// parseAPIFlags runs fs.Parse (reordering flags before any positional
// arguments first, a no-op when a command takes none) and, on success,
// resolves flags' pointers into plain values, including reconciling
// --json/--output into a single outputFlags. ok is false once fs.Parse,
// --help, or an invalid/conflicting --output has already written its
// own message to stderr (or produced exitOK for -h); the caller should
// return exitCode unchanged in that case. This is the parse-then-deref
// sequence every apiFlagSet caller needs before its own request logic
// diverges, shared here rather than repeated inline (this exact block
// was flagged as duplicated code across dozens of commands before this
// function existed).
func parseAPIFlags(fs *flag.FlagSet, args []string, flags apiFlagPtrs, prog string, stderr io.Writer) (tokenFlag, apiURLFlag, profileFlag string, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return "", "", "", false, outputFlags{}, exitOK, false
		}
		return "", "", "", false, outputFlags{}, exitUsage, false
	}
	format, ferr := resolveOutputFormat(*flags.jsonOut, *flags.output)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return "", "", "", false, outputFlags{}, exitValidation, false
	}
	return *flags.token, *flags.apiURL, *flags.profile, *flags.jsonOut, outputFlags{format, *flags.query}, 0, true
}

// singleArgCmd bundles a single-positional-argument command's own
// identity (prog, its cmdLabel, and what that one argument is called in
// a usage message, e.g. "app name" or "policy id") into one value,
// keeping parseSingleArgClient under golangci-lint's parameter-count
// limit.
type singleArgCmd struct {
	prog, cmdLabel, argLabel string
}

// parseSingleArgClient runs the parse-flags-then-require-one-arg-then-
// build-client sequence every "verb <name> [flags]" subcommand needs
// before its own request logic diverges (apps get/status/network/
// diagnose/resource-recommendation, apps git-source get/set/delete,
// databases get/resource-recommendation, iam policies get/delete/
// attachments, the identical shape many other single-argument commands
// already share). ok is false once fs.Parse, --help, or the missing-arg
// check has already written its own message to stderr; the caller
// should return exitCode unchanged in that case.
func parseSingleArgClient(fs *flag.FlagSet, args []string, flags apiFlagPtrs, stderr io.Writer, cmd singleArgCmd, lookupEnv func(string) (string, bool)) (client *Client, name string, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, flags, cmd.prog, stderr)
	if !ok {
		return nil, "", false, outputFlags{}, exitCode, false
	}

	name, ok = requireOneArg(fs, stderr, cmd.prog, cmd.cmdLabel, cmd.argLabel)
	if !ok {
		return nil, "", false, outputFlags{}, exitUsage, false
	}

	return apiClientFromFlags(cmd.prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), name, jsonOut, of, exitOK, true
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
