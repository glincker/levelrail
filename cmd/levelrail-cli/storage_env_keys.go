package main

import (
	"context"
	"fmt"
	"io"
)

// runStorageEnvKeys dispatches "storage-env-keys <verb> [flags]",
// currently just "list": GET /api/v1/storage-env-keys
// (internal/api/storage_env_keys.go's own handleListStorageEnvKeys doc
// comment), every env var name a storage-target-backed attachment can
// inject into an app's container.
func runStorageEnvKeys(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, storageEnvKeysUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, storageEnvKeysUsage(prog))
		return exitOK
	case "list":
		return runStorageEnvKeysList(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown storage-env-keys subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, storageEnvKeysUsage(prog))
		return exitUsage
	}
}

func storageEnvKeysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s storage-env-keys list [flags]   list the env var names a storage attachment can inject into a container

Run "%[1]s storage-env-keys <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runStorageEnvKeysList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "storage-env-keys list", "print env key names as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, storageEnvKeysListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	keys, err := client.ListStorageEnvKeys(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list storage env keys: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, keys, func() { printStorageEnvKeysTable(stdout, keys) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printStorageEnvKeysTable(out io.Writer, keys []string) {
	if len(keys) == 0 {
		_, _ = fmt.Fprintln(out, "no storage env keys")
		return
	}
	for _, k := range keys {
		_, _ = fmt.Fprintln(out, k)
	}
}

func storageEnvKeysListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s storage-env-keys list [flags]

Lists every env var name a storage-target-backed attachment can resolve
into an app's container (e.g. S3_ENDPOINT, S3_BUCKET).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print env key names as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
