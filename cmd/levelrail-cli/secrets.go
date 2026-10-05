package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

// runSecrets dispatches "secrets <verb> [flags]": control-plane-wide
// envelope encryption maintenance, distinct from "apps secrets".
// Rotation deliberately has no MCP tool, to keep key material out of
// agent processes; binding status reaches MCP through the doctor check.
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
	case "generate-master-key":
		return runSecretsGenerateMasterKey(prog, args[1:], stdout, stderr)
	case "binding-status":
		return runSecretsBindingStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "rebind":
		return runSecretsRebind(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown secrets subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, secretsUsage(prog))
		return exitUsage
	}
}

func secretsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets generate-master-key --out PATH [--json]
  %[1]s secrets rotate-master-key --new-key-file PATH [flags]
  %[1]s secrets binding-status [flags]
  %[1]s secrets rebind [flags]

generate-master-key writes a new master key to a file, locally, never
contacting the control plane. rotate-master-key re-wraps every stored data encryption key under a new
master key in one atomic step, live, then binds any legacy values.
binding-status counts secret values not yet bound to their slot, and
rebind binds them. Read docs/master-key-rotation.md before running these
in production.

Run "%[1]s secrets <command> -h" for its own flags.
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
  --new-key-file string    path to a file holding the new master key (a key from "secrets generate-master-key", not a plain age identity; pass "-" to read from stdin instead). Required.
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the rotation result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runSecretsRotateMasterKey(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets rotate-master-key", "print the rotation result as JSON to stdout and nothing else", stderr)
	var newKeyFile string
	fs.StringVar(&newKeyFile, "new-key-file", "", `path to a file holding the new master key (a key from "secrets generate-master-key", or an existing "master.key"); pass "-" to read from stdin instead. Required. Never pass the key itself as a bare argument, it would leak into shell history and process listings.`)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, secretsRotateMasterKeyUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

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

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printRotateMasterKeyResultHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
	if r.Rebind != nil {
		_, _ = fmt.Fprintf(out, "rebound:           %d (remaining legacy: %d, failed: %d)\n", r.Rebind.Rebound, r.Rebind.Remaining, r.Rebind.FailedCount)
	}
	if r.Warning != "" {
		_, _ = fmt.Fprintf(out, "\nWARNING: %s\n", r.Warning)
	}
}

// runSecretsGenerateMasterKey writes a fresh master key to --out (mode
// 0600, never overwriting) so rotation never needs a throwaway data dir.
func runSecretsGenerateMasterKey(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" secrets generate-master-key", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var out string
	var jsonOut bool
	fs.StringVar(&out, "out", "", "file to write the new master key to, mode 0600, must not already exist. Required.")
	fs.BoolVar(&jsonOut, "json", false, "print the result as JSON to stdout, nothing else")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if strings.TrimSpace(out) == "" {
		_, _ = fmt.Fprintf(stderr, "%s: secrets generate-master-key: --out is required\n", prog)
		return exitUsage
	}
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // operator-chosen output path
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: secrets generate-master-key: %s\n", prog, err)
		return exitUsage
	}
	if _, err := f.WriteString(mk.String() + "\n"); err != nil {
		_ = f.Close()
		_, _ = fmt.Fprintf(stderr, "%s: secrets generate-master-key: write %s: %s\n", prog, out, err)
		return exitUsage
	}
	if err := f.Close(); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: secrets generate-master-key: close %s: %s\n", prog, out, err)
		return exitUsage
	}
	if jsonOut {
		enc := json.NewEncoder(stdout)
		if err := enc.Encode(map[string]string{"path": out}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "wrote a new master key to %s (mode 0600). Next: %s secrets rotate-master-key --new-key-file %s\n", out, prog, out)
	return exitOK
}
