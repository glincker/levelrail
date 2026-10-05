package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAuthEngine dispatches "auth-engine <verb>"; status is the only verb.
func runAuthEngine(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authEngineUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, authEngineUsage(prog))
		return exitOK
	case "status":
		return runAuthEngineStatus(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown auth-engine subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, authEngineUsage(prog))
		return exitUsage
	}
}

func runAuthEngineStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth-engine status", "print the status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authEngineUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	status, err := client.GetAuthEngineStatus(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get auth engine status: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, status, func() { printAuthEngineStatus(stdout, status) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAuthEngineStatus(w io.Writer, s apiclient.AuthEngineStatusResource) {
	_, _ = fmt.Fprintf(w, "mode:             %s\n", s.Mode)
	_, _ = fmt.Fprintf(w, "library version:  %s\n", s.LibraryVersion)
	if len(s.Areas) > 0 {
		_, _ = fmt.Fprintf(w, "library areas:    %s\n", strings.Join(s.Areas, ", "))
	}
	_, _ = fmt.Fprintf(w, "compared:         %d\n", s.Compared)
	_, _ = fmt.Fprintf(w, "matched:          %d\n", s.Matched)
	_, _ = fmt.Fprintf(w, "mismatched:       %d\n", s.Mismatched)
	_, _ = fmt.Fprintf(w, "dropped:          %d\n", s.Dropped)
	for _, m := range s.Mismatches {
		_, _ = fmt.Fprintf(w, "  %s  %-9s token=%s legacy=%v library=%v\n", m.At.Format("2006-01-02 15:04:05"), m.Kind, m.TokenID, m.LegacyAbilities, m.LibraryAbilities)
	}
}

func authEngineUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth-engine status [flags]

Shows which auth engine is active, the areas served by the library, and the
shadow comparison counters with the most recent mismatches. Needs a root token.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the status as JSON to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
