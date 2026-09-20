package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runSharedEnv dispatches "shared-env <verb> [flags]" to one of
// list/set/delete: project/organization/environment-scoped variables
// every app filed under that scope inherits automatically
// (internal/reconcile/application's resolveEnv), without copy-pasting
// the same value into each app's own env editor. See
// internal/api/project_env.go, organization_env.go, environment_env.go,
// and shared_env_secrets.go for the routes these wrap.
func runSharedEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, sharedEnvUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, sharedEnvUsage(prog))
		return exitOK
	case "list":
		return runSharedEnvList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSharedEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runSharedEnvDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown shared-env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, sharedEnvUsage(prog))
		return exitUsage
	}
}

func sharedEnvUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s shared-env list   --scope SCOPE --id ID [flags]                     list shared vars, plain and secret-marked alike
  %[1]s shared-env set    --scope SCOPE --id ID <key> <value> [--secret]    set (or replace) one shared var
  %[1]s shared-env delete --scope SCOPE --id ID <key> [--secret]            remove one shared var

SCOPE is one of: project, organization, environment.

A plain shared var's value is stored as-is; --secret encrypts it the
same way an app's own { secret: true } env vars are (internal/secrets),
and its value is never shown again, only its key.

Run "%[1]s shared-env <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// sharedEnvScopeFlags adds --scope/--id to fs, shared by all three
// subcommands below.
func sharedEnvScopeFlags(fs *flag.FlagSet) (scope, id *string) {
	scope = fs.String("scope", "", "shared var scope: project, organization, or environment (required)")
	id = fs.String("id", "", "the scope's resource id (required)")
	return scope, id
}

func validateSharedEnvScope(stdout, stderr io.Writer, jsonOut bool, scope, id string) (int, bool) {
	switch scope {
	case "project", "organization", "environment":
	default:
		return reportError(stdout, stderr, jsonOut, newValidationError("--scope must be one of: project, organization, environment")), false
	}
	if id == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--id is required")), false
	}
	return exitOK, true
}

func runSharedEnvList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "shared-env list", "print shared vars as JSON to stdout and nothing else", stderr)
	scopeP, idP := sharedEnvScopeFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, sharedEnvListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if code, ok := validateSharedEnvScope(stdout, stderr, jsonOut, *scopeP, *idP); !ok {
		return code
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	vars, err := client.ListSharedEnvAll(context.Background(), *scopeP, *idP)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list shared env vars for %s %q: %w", *scopeP, *idP, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, vars, func() { printSharedEnvVarsTable(stdout, vars) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSharedEnvVarsTable(out io.Writer, vars []sharedEnvVarResource) {
	if len(vars) == 0 {
		_, _ = fmt.Fprintln(out, "no shared vars set")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tSECRET")
	for _, v := range vars {
		value := v.Value
		if v.Secret {
			value = "(hidden)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\n", v.Key, value, v.Secret)
	}
	_ = tw.Flush()
}

func sharedEnvListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s shared-env list --scope SCOPE --id ID [flags]

Lists every shared var at the given scope, plain and secret-marked
alike. A secret-marked entry's value is never shown.

Flags:
  --scope string          project, organization, or environment (required)
  --id string              the scope's resource id (required)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print shared vars as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runSharedEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, _, _ := apiFlagSet(prog, "shared-env set", "unused for this subcommand", stderr)
	scopeP, idP := sharedEnvScopeFlags(fs)
	var secret bool
	fs.BoolVar(&secret, "secret", false, "encrypt the value (internal/secrets), never shown again")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s shared-env set --scope SCOPE --id ID <key> <value> [--secret] [flags]\n\nSets (or replaces) one shared var's value.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if code, ok := validateSharedEnvScope(stdout, stderr, jsonOut, *scopeP, *idP); !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "shared-env set", "a key and a value", 2)
	if !ok {
		return exitUsage
	}
	key, value := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if secret {
		if err := client.SetSharedEnvSecret(ctx, *scopeP, *idP, key, value); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set secret shared var %q for %s %q: %w", key, *scopeP, *idP, err))
		}
		_, _ = fmt.Fprintf(stdout, "secret shared var %q set for %s %q\n", key, *scopeP, *idP)
		return exitOK
	}

	// Plain vars have no single-key upsert route (store.DB.
	// Set{Project,Organization,Environment}EnvVars is a full replace by
	// design): read the current set, merge this key in, write the whole
	// map back, the same read-merge-write shape those store functions'
	// own doc comments already document for exactly this case.
	current, err := getPlainSharedEnv(ctx, client, *scopeP, *idP)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read current shared vars for %s %q: %w", *scopeP, *idP, err))
	}
	current[key] = value
	if err := setPlainSharedEnv(ctx, client, *scopeP, *idP, current); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set shared var %q for %s %q: %w", key, *scopeP, *idP, err))
	}
	_, _ = fmt.Fprintf(stdout, "shared var %q set for %s %q\n", key, *scopeP, *idP)
	return exitOK
}

func runSharedEnvDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, _, _ := apiFlagSet(prog, "shared-env delete", "unused for this subcommand", stderr)
	scopeP, idP := sharedEnvScopeFlags(fs)
	var secret bool
	fs.BoolVar(&secret, "secret", false, "the key is secret-marked, not a plain var")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s shared-env delete --scope SCOPE --id ID <key> [--secret] [flags]\n\nRemoves one shared var.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if code, ok := validateSharedEnvScope(stdout, stderr, jsonOut, *scopeP, *idP); !ok {
		return code
	}
	key, ok := requireOneArg(fs, stderr, prog, "shared-env delete", "key")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if secret {
		if err := client.DeleteSharedEnvSecret(ctx, *scopeP, *idP, key); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete secret shared var %q for %s %q: %w", key, *scopeP, *idP, err))
		}
		_, _ = fmt.Fprintf(stdout, "secret shared var %q removed for %s %q\n", key, *scopeP, *idP)
		return exitOK
	}

	current, err := getPlainSharedEnv(ctx, client, *scopeP, *idP)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read current shared vars for %s %q: %w", *scopeP, *idP, err))
	}
	delete(current, key)
	if err := setPlainSharedEnv(ctx, client, *scopeP, *idP, current); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete shared var %q for %s %q: %w", key, *scopeP, *idP, err))
	}
	_, _ = fmt.Fprintf(stdout, "shared var %q removed for %s %q\n", key, *scopeP, *idP)
	return exitOK
}

func getPlainSharedEnv(ctx context.Context, client *Client, scope, id string) (map[string]string, error) {
	switch scope {
	case "project":
		return client.GetProjectEnv(ctx, id)
	case "organization":
		return client.GetOrganizationEnv(ctx, id)
	default:
		return client.GetEnvironmentEnv(ctx, id)
	}
}

func setPlainSharedEnv(ctx context.Context, client *Client, scope, id string, vars map[string]string) error {
	var err error
	switch scope {
	case "project":
		_, err = client.SetProjectEnv(ctx, id, vars)
	case "organization":
		_, err = client.SetOrganizationEnv(ctx, id, vars)
	default:
		_, err = client.SetEnvironmentEnv(ctx, id, vars)
	}
	return err
}
