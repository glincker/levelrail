package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runMe implements "me appearance": the signed-in user's own dashboard look,
// synced through /api/v1/me/preferences/appearance.
func runMe(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, meUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "appearance":
		return runMeAppearance(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown me command %q\n\n%s", prog, args[0], meUsage(prog))
		return exitUsage
	}
}

func runMeAppearance(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	sub := "get"
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "get", "set", "reset":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown me appearance command %q\n\n%s", prog, sub, meUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "me appearance "+sub, "print the appearance settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, meUsage(prog)) }
	confirm := fs.Bool("yes", false, "confirm a reset (required for me appearance reset)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if sub == "reset" && !*confirm {
		_, _ = fmt.Fprintf(stderr, "%s: me appearance reset clears your saved look on every browser; pass --yes to confirm\n", prog)
		return exitUsage
	}
	var doc json.RawMessage
	if sub == "set" {
		if len(rest) != 1 {
			_, _ = fmt.Fprintf(stderr, "%s: me appearance set needs one file path or - for stdin\n", prog)
			return exitUsage
		}
		raw, err := readAppearanceInput(rest[0], stdin)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
			return exitUsage
		}
		if !json.Valid(raw) {
			_, _ = fmt.Fprintf(stderr, "%s: appearance input is not valid JSON\n", prog)
			return exitUsage
		}
		doc = raw
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		p   apiclient.MePreference
		err error
	)
	switch sub {
	case "set":
		p, err = client.SetMeAppearance(ctx, doc)
	case "reset":
		p, err = client.ResetMeAppearance(ctx)
	default:
		p, err = client.GetMeAppearance(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("me appearance %s: %w", sub, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() {
		if string(p.Value) == "null" || len(p.Value) == 0 {
			_, _ = fmt.Fprintln(stdout, "No appearance saved to your account.")
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", p.Value)
	})
}

func readAppearanceInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(io.LimitReader(stdin, 1<<20))
	}
	return os.ReadFile(path) //nolint:gosec // user-chosen file by design
}

func meUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s me appearance [get] [flags]           show your appearance saved to your account
  %[1]s me appearance set <file|-> [flags]    save a JSON object (file or stdin) as your appearance
  %[1]s me appearance reset --yes [flags]     clear your saved appearance

Flags:
  --yes               confirm a reset
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the result as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
