package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

func runAppsSecrets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsSecretsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsSecretsUsage(prog))
		return exitOK
	case "list":
		return runAppsSecretsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsSecretsSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsSecretsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "lock":
		return runAppsSecretsLock(prog, args[1:], stdout, stderr, lookupEnv)
	case "build-time":
		return runAppsSecretsBuildTime(prog, args[1:], stdout, stderr, lookupEnv)
	case "rotation":
		return runAppsSecretsRotation(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps secrets subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsSecretsUsage(prog))
		return exitUsage
	}
}

func appsSecretsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps secrets list <name> [flags]                          list an app's secret keys and their locked state
  %[1]s apps secrets set <name> <key> <value> [flags]              set (or rotate) one secret's value
  %[1]s apps secrets set <name> --env-file <path> [flags]           bulk-set every key in a .env-format file as a secret
  %[1]s apps secrets delete <name> <key> [--force] [flags]         delete a secret's value and stop injecting it
  %[1]s apps secrets lock <name> <key> --locked=true|false [flags]  toggle a secret's overwrite guard
  %[1]s apps secrets build-time <name> <key> --enabled=true|false [flags]  allow or deny a secret in app.yaml build.secrets
  %[1]s apps secrets rotation <list|due|set|remove|rotate> ...      scheduled rotation policies

A secret you set is injected into the app's container from the next restart
on. Pass --apply to set to restart right away.

Values are never returned: list shows key names and locked state only,
matching internal/api/secrets.go's own "never echo a value back" rule.

Run "%[1]s apps secrets <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsSecretsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps secrets list", "print the secret keys as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets list <name> [flags]\n\nLists an app's secret keys and their locked state. Never a value.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps secrets list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	keys, err := client.ListSecrets(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list secrets for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, keys, func() { printSecretKeysHuman(stdout, keys) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsSecretsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets set", "unused for this subcommand", stderr)
	var overwriteLocked bool
	var envFile string
	var apply, buildTime bool
	fs.BoolVar(&apply, "apply", false, "restart the app now so the new value reaches the running container")
	fs.BoolVar(&buildTime, "build-time", false, "also enable the secret for app.yaml build.secrets (off by default); a Dockerfile can read it during the build")
	fs.BoolVar(&overwriteLocked, "force", false, "overwrite the value even if the key is locked")
	fs.StringVar(&envFile, "env-file", "", "bulk-set every key in this .env-format file as a secret, instead of a single key/value pair")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets set <name> <key> <value> [--build-time] [flags]\n  %s apps secrets set <name> --env-file <path> [--build-time] [flags]\n\nSets (or rotates) one secret's encrypted value, or every key in a\n.env-format file as its own secret. Values are never printed back.\n--build-time also allows app.yaml build.secrets to mount the secret.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag := *tokenFlagP, *apiURLFlagP, *profileFlagP
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if envFile != "" {
		rest, ok := requireArgs(fs, stderr, prog, "apps secrets set", "an app name", 1)
		if !ok {
			return exitUsage
		}
		code := runAppsSecretsSetEnvFile(client, rest[0], envFile, overwriteLocked, buildTime, stdout, stderr)
		if code == exitOK {
			code = afterConfigWrite(context.Background(), client, prog, rest[0], apply, stdout, stderr)
		}
		return code
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps secrets set", "an app name, a key, and a value", 3)
	if !ok {
		return exitUsage
	}
	name, key, value := rest[0], rest[1], rest[2]

	if err := setSecretMaybeBuildTime(client, name, key, value, overwriteLocked, buildTime); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("set secret %q for app %q: %w", key, name, err))
	}
	_, _ = fmt.Fprintf(stdout, "secret %q set for app %q\n", key, name)
	return afterConfigWrite(context.Background(), client, prog, name, apply, stdout, stderr)
}

func runAppsSecretsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets delete", "unused for this subcommand", stderr)
	var force, apply bool
	fs.BoolVar(&force, "force", false, "delete the value even if the key is locked")
	fs.BoolVar(&apply, "apply", false, "restart the app now so the removal reaches the running container")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets delete <name> <key> [flags]\n\nDeletes a secret's stored value and stops declaring the key as\nsecret-backed, so it is no longer injected.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps secrets delete", "an app name and a key", 2)
	if !ok {
		return exitUsage
	}
	name, key := rest[0], rest[1]
	client := apiClientFromFlags(prog, *apiURLFlagP, *tokenFlagP, *profileFlagP, lookupEnv)
	if err := client.DeleteSecret(context.Background(), name, key, force); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("delete secret %q for app %q: %w", key, name, err))
	}
	_, _ = fmt.Fprintf(stdout, "secret %q deleted for app %q\n", key, name)
	return afterConfigWrite(context.Background(), client, prog, name, apply, stdout, stderr)
}

// Only key names are printed, never values.
func runAppsSecretsSetEnvFile(client *Client, name, path string, overwriteLocked, buildTime bool, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied local path, same trust boundary as every other --file flag in this CLI
	if err != nil {
		return reportError(stdout, stderr, false, newValidationError("read env file %q: %v", path, err))
	}

	entries := parseEnvFileBytes(data)
	if len(entries) == 0 {
		_, _ = fmt.Fprintf(stdout, "no keys found in %q, nothing set\n", path)
		return exitOK
	}

	var failedKeys []string
	for _, entry := range entries {
		if err := setSecretMaybeBuildTime(client, name, entry.Key, entry.Value, overwriteLocked, buildTime); err != nil {
			_, _ = fmt.Fprintf(stderr, "secret %q for app %q: %v\n", entry.Key, name, err)
			failedKeys = append(failedKeys, entry.Key)
			continue
		}
		_, _ = fmt.Fprintf(stdout, "secret %q set for app %q\n", entry.Key, name)
	}
	if len(failedKeys) > 0 {
		_, _ = fmt.Fprintf(stderr, "%d of %d secret(s) failed: %s\n", len(failedKeys), len(entries), strings.Join(failedKeys, ", "))
		return exitAPIError
	}
	return exitOK
}

// setSecretMaybeBuildTime only sends build_time when --build-time was given,
// so a plain set never changes an existing secret's build access.
func setSecretMaybeBuildTime(client *Client, name, key, value string, overwriteLocked, buildTime bool) error {
	if buildTime {
		return client.SetSecretWithBuildTime(context.Background(), name, key, value, overwriteLocked, true)
	}
	return client.SetSecret(context.Background(), name, key, value, overwriteLocked)
}

func runAppsSecretsBuildTime(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets build-time", "unused for this subcommand", stderr)
	var enabled bool
	fs.BoolVar(&enabled, "enabled", true, "true to let app.yaml build.secrets mount the key, false to deny it again")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets build-time <name> <key> [--enabled=true|false] [flags]\n\nAllows or denies a secret in app.yaml build.secrets. Off by default. An enabled\nsecret can be read by anyone who can change the Dockerfile.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps secrets build-time", "an app name and a key", 2)
	if !ok {
		return exitUsage
	}
	name, key := rest[0], rest[1]
	client := apiClientFromFlags(prog, *apiURLFlagP, *tokenFlagP, *profileFlagP, lookupEnv)
	if err := client.SetSecretBuildTime(context.Background(), name, key, enabled); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("set build-time access for secret %q on app %q: %w", key, name, err))
	}
	state := "enabled"
	if !enabled {
		state = "disabled"
	}
	_, _ = fmt.Fprintf(stdout, "secret %q on app %q %s for build use\n", key, name, state)
	return exitOK
}

func runAppsSecretsLock(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets lock", "unused for this subcommand", stderr)
	var locked bool
	fs.BoolVar(&locked, "locked", true, "true to lock the key against overwrite, false to unlock it")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets lock <name> <key> [--locked=true|false] [flags]\n\nToggles a secret's accidental-overwrite guard. Reversible either\ndirection, not a permanent write-once marker.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag := *tokenFlagP, *apiURLFlagP, *profileFlagP

	rest, ok := requireArgs(fs, stderr, prog, "apps secrets lock", "an app name and a key", 2)
	if !ok {
		return exitUsage
	}
	name, key := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.SetSecretLock(context.Background(), name, key, locked); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("set lock for secret %q on app %q: %w", key, name, err))
	}
	state := "locked"
	if !locked {
		state = "unlocked"
	}
	_, _ = fmt.Fprintf(stdout, "secret %q on app %q %s\n", key, name, state)
	return exitOK
}

func printSecretKeysHuman(out io.Writer, keys []secretKeyResource) {
	if len(keys) == 0 {
		_, _ = fmt.Fprintln(out, "no secrets set")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tLOCKED\tAGE\tSTALE\tBUILD")
	for _, k := range keys {
		_, _ = fmt.Fprintf(tw, "%s\t%t\t%s\t%t\t%t\n", k.Key, k.Locked, formatSecretAge(k.UpdatedAt), k.Stale, k.BuildTime)
	}
	_ = tw.Flush()
}
