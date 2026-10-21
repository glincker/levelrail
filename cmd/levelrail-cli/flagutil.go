package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
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

// Value is a path#field reference into Vault, never a secret value itself.
type vaultEnvFlag map[string]appVaultEnvRef

func (m vaultEnvFlag) String() string {
	if len(m) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, k+"="+v.Path+"#"+v.Key)
	}
	return strings.Join(pairs, ",")
}

// Splits on the last '#' so a Vault KV path containing '#' stays intact.
func (m vaultEnvFlag) Set(s string) error {
	key, ref, ok := strings.Cut(s, "=")
	if !ok || key == "" {
		return fmt.Errorf("invalid KEY=path#field %q, want a key, an \"=\", and a path#field value", s)
	}
	sep := strings.LastIndex(ref, "#")
	if sep < 0 {
		return fmt.Errorf("invalid vault reference %q, want \"path#field\"", ref)
	}
	path, field := ref[:sep], ref[sep+1:]
	if path == "" || field == "" {
		return fmt.Errorf("invalid vault reference %q, want \"path#field\"", ref)
	}
	m[key] = appVaultEnvRef{Path: path, Key: field}
	return nil
}

// Ordered list, not a map: an egress allowlist is a sequence of entries.
type egressAllowFlag []appEgressAllow

func (f *egressAllowFlag) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	parts := make([]string, len(*f))
	for i, a := range *f {
		parts[i] = fmt.Sprintf("%s:%d", a.Host, a.Port)
	}
	return strings.Join(parts, ",")
}

// Splits on the last ':' so hosts never need IPv6 special-casing, matching the egress.allow schema.
func (f *egressAllowFlag) Set(s string) error {
	sep := strings.LastIndex(s, ":")
	if sep < 0 {
		return fmt.Errorf("invalid host:port %q, want a host, a \":\", and a port", s)
	}
	host, portStr := s[:sep], s[sep+1:]
	if host == "" {
		return fmt.Errorf("invalid host:port %q, host must not be empty", s)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid host:port %q, port must be a number between 1 and 65535", s)
	}
	*f = append(*f, appEgressAllow{Host: host, Port: port})
	return nil
}

// --json stays as shorthand for --output json so existing scripts keep working.
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

// Split out so hand-built FlagSets can share the output flags without duplicating descriptions.
func bindOutputQueryFlags(fs *flag.FlagSet) (outputFlag, queryFlag *string) {
	outputFlag = fs.String("output", "", "output format: json, table, or text (default table; --json is shorthand for --output json)")
	queryFlag = fs.String("query", "", "JMESPath expression to filter the result before printing (e.g. \"[?status=='running'].name\")")
	return outputFlag, queryFlag
}

func apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag string, lookupEnv func(string) (string, bool)) *Client {
	profile := resolveProfile(profileFlag, lookupEnv)
	applyScope(prog, profile, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	if token == "" && firstRunInteractive() {
		if u, t, ok := offerFirstRunLogin(prog, profile, apiURL, lookupEnv, os.Stdin, os.Stdout, os.Stderr); ok {
			apiURL, token = u, t
		}
	}
	if token == "" {
		return NewClient(apiURL, token, apiclient.WithNoCredentialsFix(notSignedInFix(prog, profile, apiURL)))
	}
	return NewClient(apiURL, token)
}

// Bundled to keep parameter counts under golangci-lint's limit.
type credentialFlags struct {
	Token, APIURL, Profile string
}

// apiClientFromCredentialFlags is apiClientFromFlags taking cf as one
// value instead of three separate strings.
func apiClientFromCredentialFlags(prog string, cf credentialFlags, lookupEnv func(string) (string, bool)) *Client {
	return apiClientFromFlags(prog, cf.APIURL, cf.Token, cf.Profile, lookupEnv)
}

// Session-auth counterpart of apiFlagSet; these commands never accept bearer tokens.
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

// Returns the login response too, since callers like auth login print the username from it.
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
	if err == nil && loginResp.ApprovalRequired {
		loginResp, err = sessionClient.awaitApproval(ctx, prog, loginResp, lookupEnv, stderr)
	}
	if err != nil {
		return nil, loginResponse{}, fmt.Errorf("log in as %q: %w", resolvedUsername, err)
	}
	return sessionClient, loginResp, nil
}

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

// Pointers are read only after fs.Parse succeeds.
type apiFlagPtrs struct {
	token, apiURL, profile *string
	jsonOut                *bool
	output, query          *string
}

// Format already folds in the legacy --json flag, see resolveOutputFormat.
type outputFlags struct {
	Format outputFormat
	Query  string
}

// ok is false when parsing, --help, or a bad --output already wrote to stderr; return exitCode unchanged.
func parseAPIFlags(fs *flag.FlagSet, args []string, flags apiFlagPtrs, prog string, stderr io.Writer) (tokenFlag, apiURLFlag, profileFlag string, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
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

// Same contract as parseAPIFlags: ok false means the caller returns exitCode unchanged.
func parseSessionFlags(fs *flag.FlagSet, args []string, jsonOutP *bool, outputFlagP, queryFlagP *string, prog string, stderr io.Writer) (jsonOut bool, of outputFlags, exitCode int, ok bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return false, outputFlags{}, exitOK, false
		}
		return false, outputFlags{}, exitUsage, false
	}
	jsonOut = *jsonOutP
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return false, outputFlags{}, exitValidation, false
	}
	return jsonOut, outputFlags{format, *queryFlagP}, 0, true
}

// ok is false once the login failure has been reported; return exitCode unchanged.
func buildSessionClient(ctx context.Context, sf sessionFlags, prog string, lookupEnv func(string) (string, bool), stdin io.Reader, stdout, stderr io.Writer, jsonOut bool) (client *authSessionClient, exitCode int, ok bool) {
	sessionClient, _, err := loggedInSessionClient(ctx, sf, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return nil, reportError(stdout, stderr, jsonOut, err), false
	}
	return sessionClient, 0, true
}

type sessionFlagPtrs struct {
	username, password, apiURL, profile *string
	jsonOut                             *bool
	output, query                       *string
}

type singleArgCmd struct {
	prog, cmdLabel, argLabel string
}

// ok is false once parsing or the missing-arg check has reported; return exitCode unchanged.
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

type twoArgCmd struct {
	prog, cmdLabel, argsLabel string
}

// Same ok contract as parseSingleArgClient.
func parseTwoArgClient(fs *flag.FlagSet, args []string, flags apiFlagPtrs, stderr io.Writer, cmd twoArgCmd, lookupEnv func(string) (string, bool)) (client *Client, first, second string, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, flags, cmd.prog, stderr)
	if !ok {
		return nil, "", "", false, outputFlags{}, exitCode, false
	}

	positional, ok := requireArgs(fs, stderr, cmd.prog, cmd.cmdLabel, cmd.argsLabel, 2)
	if !ok {
		return nil, "", "", false, outputFlags{}, exitUsage, false
	}

	return apiClientFromFlags(cmd.prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), positional[0], positional[1], jsonOut, of, exitOK, true
}

// Same ok contract as parseSingleArgClient.
func parseListClient(fs *flag.FlagSet, args []string, flags apiFlagPtrs, prog string, stderr io.Writer, lookupEnv func(string) (string, bool)) (client *Client, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, flags, prog, stderr)
	if !ok {
		return nil, false, outputFlags{}, exitCode, false
	}
	return apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), jsonOut, of, exitOK, true
}

// flag.FlagSet stops parsing at the first positional, so 'apps get web --json' would silently ignore --json.
// fs must define all flags first, so boolean flags can be told apart from value flags.
// This is why the rewrite is needed at all.
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
