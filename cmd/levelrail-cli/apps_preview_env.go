package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsPreviewEnv dispatches "apps preview-env <verb> [flags]" to one
// of set/clear: PUT/DELETE /api/v1/apps/{name}/preview-env/{key}
// (internal/api/apps_preview_env.go), declaring or removing one env
// var's preview-specific value on an app that already exists. Mirrors
// runAppsVaultEnv's own set/clear dispatch shape: one entry per env var
// key, both subcommands take an env var key as a second positional
// argument.
func runAppsPreviewEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsPreviewEnvUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsPreviewEnvUsage(prog))
		return exitOK
	case "set":
		return runAppsPreviewEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsPreviewEnvClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps preview-env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsPreviewEnvUsage(prog))
		return exitUsage
	}
}

func appsPreviewEnvUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps preview-env set <name> <key> --value VALUE [flags]   declare (or replace) a preview-specific env var override
  %[1]s apps preview-env clear <name> <key> [flags]                remove a preview-specific env var override

Declares one env var to take a different value only when a preview
environment is next created from app <name>, replacing whatever that key
would otherwise inherit (a plain value, a secret, or a Vault reference).
Never affects <name>'s own running deploy, only a preview created from it
afterward.

Run "%[1]s apps preview-env <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsPreviewEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps preview-env set", "print the resulting {key,value} as JSON to stdout and nothing else", stderr)
	var value string
	fs.StringVar(&value, "value", "", "the preview-specific value for this env var (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps preview-env set <name> <key> --value VALUE [flags]\n\nDeclares <key>'s preview-specific value on app <name>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	positional, ok := requireArgs(fs, stderr, prog, "apps preview-env set", "an app name and an env var key", 2)
	if !ok {
		return exitUsage
	}
	name, key := positional[0], positional[1]
	if value == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps preview-env set requires --value\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.SetAppPreviewEnvOverride(context.Background(), name, key, value)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set preview-env %q for app %q: %w", key, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "%s: value=%s\n", result.Key, result.Value)
	})
}

func runAppsPreviewEnvClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps preview-env clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps preview-env clear <name> <key> [flags]\n\nRemoves <key>'s preview-specific value override from app <name>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	positional, ok := requireArgs(fs, stderr, prog, "apps preview-env clear", "an app name and an env var key", 2)
	if !ok {
		return exitUsage
	}
	name, key := positional[0], positional[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.ClearAppPreviewEnvOverride(context.Background(), name, key); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear preview-env %q for app %q: %w", key, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "preview-env %q cleared for app %q\n", key, name)
	})
}
