package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func appsProtectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps protect <app-name> --reason TEXT [flags]
  %[1]s apps protect <app-name> --status [flags]
  %[1]s apps unprotect <app-name> [flags]

A protected app refuses deploys, scaling, config writes and deletion with
HTTP 423 until it is unprotected. Reads, logs and metrics are unaffected.
Unprotecting is recorded in the app's event history. This is separate from
"apps lock", which only holds new builds back from going live.

Flags:
  --reason string         why the app is protected (required)
  --status                show the protection without changing it
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read
  --json                  print the result as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

// runAppsProtect implements "apps protect" and "apps unprotect".
func runAppsProtect(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var reason string
	var status bool
	if verb == "protect" {
		fs.StringVar(&reason, "reason", "", "why the app is protected")
		fs.BoolVar(&status, "status", false, "show the protection without changing it")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsProtectUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	if verb == "protect" && !status && strings.TrimSpace(reason) == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s requires --reason", label))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var err error
	var res = struct{ Protected bool }{}
	switch {
	case verb == "unprotect":
		var r2, e = client.UnprotectApp(ctx, name)
		res.Protected, err = r2.Protected, e
	case status:
		var r2, e = client.GetAppProtection(ctx, name)
		res.Protected, err = r2.Protected, e
	default:
		var r2, e = client.ProtectApp(ctx, name, strings.TrimSpace(reason))
		res.Protected, err = r2.Protected, e
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s %q: %w", label, name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		state := "not protected"
		if res.Protected {
			state = "protected"
		}
		_, _ = fmt.Fprintf(stdout, "app %q is %s\n", name, state)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
