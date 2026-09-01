package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// runSecrets dispatches "secrets <verb> [flags]", currently just
// rotate-master-key. Distinct from "apps secrets" (apps.go), which
// manages one app's own env-var secret values: this is the control
// plane's single envelope-encryption master key, not scoped to any app.
func runSecrets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, secretsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, secretsUsage(prog))
		return exitOK
	case "rotate-master-key":
		return runSecretsRotateMasterKey(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown secrets subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, secretsUsage(prog))
		return exitUsage
	}
}

func secretsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets rotate-master-key --new-key-file PATH [flags]

Rotates the control plane's envelope-encryption master key: every
stored per-app data encryption key is re-wrapped under a new master key
in one atomic step, live, while the control plane keeps serving.
Read docs/master-key-rotation.md before running this in production.

Run "%[1]s secrets rotate-master-key -h" for its own flags.
`, prog)
}

func secretsRotateMasterKeyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets rotate-master-key --new-key-file PATH [flags]

Re-wraps every stored data encryption key from the currently active
master key to the new one, in a single atomic operation on the control
plane. Reads the new key from a file (or stdin with -) so it never
appears as a bare command-line argument. See docs/master-key-rotation.md
for the full procedure, including what to do next if the master key is
sourced from APP_MASTER_KEY rather than a file.

Flags:
  --new-key-file string    path to a file holding the new master key (an age identity string; pass "-" to read from stdin instead). Required.
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the rotation result as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runSecretsRotateMasterKey(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "secrets rotate-master-key", "print the rotation result as JSON to stdout and nothing else", stderr)
	var newKeyFile string
	fs.StringVar(&newKeyFile, "new-key-file", "", `path to a file holding the new master key (an age identity string, e.g. a fresh "master.key" or the output of generating one); pass "-" to read from stdin instead. Required. Never pass the key itself as a bare argument, it would leak into shell history and process listings.`)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, secretsRotateMasterKeyUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if strings.TrimSpace(newKeyFile) == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--new-key-file is required"))
	}

	newKey, err := readSecretFileOrStdin(newKeyFile)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("read --new-key-file: %s", err))
	}
	newKey = strings.TrimSpace(newKey)
	if newKey == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--new-key-file is empty"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.RotateMasterKey(context.Background(), newKey)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate master key: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, result); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printRotateMasterKeyResultHuman(stdout, result)
	return exitOK
}

// readSecretFileOrStdin reads path's contents, or stdin when path is
// "-": the file-or-stdin input shape --new-key-file needs so the new
// master key is never typed as a bare argument.
func readSecretFileOrStdin(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator-supplied flag value, this CLI's entire job is reading operator-specified files
	if err != nil {
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(data), nil
}

func printRotateMasterKeyResultHuman(out io.Writer, r rotateMasterKeyResult) {
	_, _ = fmt.Fprintf(out, "rotated_at:        %s\n", r.RotatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(out, "persisted_to_file: %t\n", r.PersistedToFile)
	if r.Warning != "" {
		_, _ = fmt.Fprintf(out, "\nWARNING: %s\n", r.Warning)
	}
}
