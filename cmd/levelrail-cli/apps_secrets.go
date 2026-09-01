package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsSecrets dispatches "apps secrets <verb> [flags]" to one of
// list/set/lock, mirroring runAppsLogDrain's own dispatch shape. Standalone
// commands for internal/api/secrets.go's three routes: before this, PUT
// .../secrets/{key} was only reachable via "migrate coolify apply"'s
// import flow, and GET .../secrets / POST .../secrets/{key}/lock had no
// CLI command at all.
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
	case "lock":
		return runAppsSecretsLock(prog, args[1:], stdout, stderr, lookupEnv)
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
  %[1]s apps secrets lock <name> <key> --locked=true|false [flags]  toggle a secret's overwrite guard

Values are never returned: list shows key names and locked state only,
matching internal/api/secrets.go's own "never echo a value back" rule.

Run "%[1]s apps secrets <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsSecretsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps secrets list", "print the secret keys as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets list <name> [flags]\n\nLists an app's secret keys and their locked state. Never a value.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, exitCode, ok := parseSingleArgClient(fs, args, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, stderr, prog, "apps secrets list", lookupEnv)
	if !ok {
		return exitCode
	}

	keys, err := client.ListSecrets(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list secrets for app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, keys); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printSecretKeysHuman(stdout, keys)
	return exitOK
}

func runAppsSecretsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _ := apiFlagSet(prog, "apps secrets set", "unused for this subcommand", stderr)
	var overwriteLocked bool
	fs.BoolVar(&overwriteLocked, "force", false, "overwrite the value even if the key is locked")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets set <name> <key> <value> [flags]\n\nSets (or rotates) one secret's encrypted value.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag := *tokenFlagP, *apiURLFlagP, *profileFlagP

	rest, ok := requireArgs(fs, stderr, prog, "apps secrets set", "an app name, a key, and a value", 3)
	if !ok {
		return exitUsage
	}
	name, key, value := rest[0], rest[1], rest[2]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.SetSecret(context.Background(), name, key, value, overwriteLocked); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("set secret %q for app %q: %w", key, name, err))
	}
	_, _ = fmt.Fprintf(stdout, "secret %q set for app %q\n", key, name)
	return exitOK
}

func runAppsSecretsLock(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _ := apiFlagSet(prog, "apps secrets lock", "unused for this subcommand", stderr)
	var locked bool
	fs.BoolVar(&locked, "locked", true, "true to lock the key against overwrite, false to unlock it")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets lock <name> <key> [--locked=true|false] [flags]\n\nToggles a secret's accidental-overwrite guard. Reversible either\ndirection, not a permanent write-once marker.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
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
	_, _ = fmt.Fprintln(tw, "KEY\tLOCKED")
	for _, k := range keys {
		_, _ = fmt.Fprintf(tw, "%s\t%t\n", k.Key, k.Locked)
	}
	_ = tw.Flush()
}
