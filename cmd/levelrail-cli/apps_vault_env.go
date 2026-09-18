package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsVaultEnv dispatches "apps vault-env <verb> [flags]" to one of
// set/clear: PUT/DELETE /api/v1/apps/{name}/vault-env/{key}
// (internal/api/apps_vault_env.go), declaring or removing one env var as
// resolving live from the platform's configured external Vault instance,
// for an app that already exists. Mirrors runAppsStorage's own set/clear
// dispatch shape; unlike storage there is one entry per env var key, not
// a single attachment, so both subcommands take an env var key as a
// second positional argument.
func runAppsVaultEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsVaultEnvUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsVaultEnvUsage(prog))
		return exitOK
	case "set":
		return runAppsVaultEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsVaultEnvClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps vault-env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsVaultEnvUsage(prog))
		return exitUsage
	}
}

func appsVaultEnvUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps vault-env set <name> <key> --path PATH --key FIELD [flags]   declare (or replace) a Vault-sourced env var
  %[1]s apps vault-env clear <name> <key> [flags]                        remove a Vault-sourced env var declaration

Declares one env var to resolve live from the control plane's configured
external Vault instance ("%[1]s vault set") instead of a value this
platform stores, the post-create equivalent of "apps create"'s own
--vault-secret flag and app.yaml's { vault: { path, key } } syntax. No
value is stored here, only the reference: the actual secret is read fresh
from Vault immediately before the app's container is next created.

Run "%[1]s apps vault-env <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// parseAppsVaultEnvArgs parses the API flags plus the <name> <key>
// positional pair shared by "apps vault-env set" and "apps vault-env
// clear", which otherwise differ only in cmdLabel and usage text.
func parseAppsVaultEnvArgs(fs *flag.FlagSet, args []string, ptrs apiFlagPtrs, prog, cmdLabel string, stderr io.Writer) (tokenFlag, apiURLFlag, profileFlag, name, key string, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok = parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return
	}
	positional, argsOK := requireArgs(fs, stderr, prog, cmdLabel, "an app name and an env var key", 2)
	if !argsOK {
		return tokenFlag, apiURLFlag, profileFlag, "", "", jsonOut, of, exitUsage, false
	}
	return tokenFlag, apiURLFlag, profileFlag, positional[0], positional[1], jsonOut, of, exitCode, true
}

func runAppsVaultEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps vault-env set", "print the resulting {path,key} as JSON to stdout and nothing else", stderr)
	var path, field string
	fs.StringVar(&path, "path", "", "Vault KV v2 secret path, e.g. myapp/config (required)")
	fs.StringVar(&field, "key", "", "field name within that secret's data (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps vault-env set <name> <key> --path PATH --key FIELD [flags]\n\nDeclares <key> as a Vault-sourced env var on app <name>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, name, key, jsonOut, of, exitCode, ok := parseAppsVaultEnvArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "apps vault-env set", stderr)
	if !ok {
		return exitCode
	}
	if path == "" || field == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps vault-env set requires --path and --key\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.SetAppVaultEnv(context.Background(), name, key, appVaultEnvRef{Path: path, Key: field})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set vault-env %q for app %q: %w", key, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "%s: path=%s key=%s\n", key, result.Path, result.Key)
	})
}

func runAppsVaultEnvClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps vault-env clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps vault-env clear <name> <key> [flags]\n\nRemoves <key>'s Vault-sourced env var declaration from app <name>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, name, key, jsonOut, of, exitCode, ok := parseAppsVaultEnvArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "apps vault-env clear", stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.ClearAppVaultEnv(context.Background(), name, key); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear vault-env %q for app %q: %w", key, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "vault-env %q cleared for app %q\n", key, name)
	})
}
