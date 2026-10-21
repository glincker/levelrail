package main

import (
	"context"
	"fmt"
	"io"
)

// Top-level because it reports the control plane's own Docker reachability, not a per-node signal.
func runStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "silence":
			return runStatusSilence(prog, args[1:], stdout, stderr, lookupEnv)
		case "silences":
			return runStatusSilences(prog, args[1:], stdout, stderr, lookupEnv)
		case "unsilence":
			return runStatusUnsilence(prog, args[1:], stdout, stderr, lookupEnv)
		}
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "status", "print status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, statusUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	status, err := client.GetSystemStatus(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get system status: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, status, func() { printSystemStatusHuman(stdout, status) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func statusUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s status [flags]

Shows this control plane's own configured/not-configured signals,
including whether its local Docker daemon is currently reachable.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print status as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
