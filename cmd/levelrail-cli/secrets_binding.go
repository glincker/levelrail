package main

import (
	"context"
	"fmt"
	"io"
)

func secretsBindingUsage(prog, verb, summary string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets %[2]s [flags]

%[3]s

Flags:
  --token string           API token (default: %[4]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[5]s env var, then %[6]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, verb, summary, envAPIToken, envAPIURL, defaultAPIURL)
}

const (
	bindingStatusHelp = `Shows how many stored secret values still use the legacy format that is
not bound to its (owner, key) slot. Run "secrets rebind" until legacy is 0.`
	rebindHelp = `Re-encrypts every legacy secret value so it only decrypts in its own
(owner, key) slot. Safe to rerun: bound values are skipped, and an
interrupted run keeps its progress. Requires the root ability.`
)

func runSecretsBindingStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets binding-status", "print the status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, secretsBindingUsage(prog, "binding-status", bindingStatusHelp)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	status, err := client.GetSecretBinding(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get secret binding status: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, status, func() { printSecretBindingStatusHuman(stdout, status) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSecretsRebind(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets rebind", "print the rebind result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, secretsBindingUsage(prog, "rebind", rebindHelp)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.RebindSecrets(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rebind secrets: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, result, func() { printSecretRebindResultHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSecretBindingStatusHuman(out io.Writer, s secretBindingStatus) {
	_, _ = fmt.Fprintf(out, "total:  %d\n", s.Total)
	_, _ = fmt.Fprintf(out, "bound:  %d\n", s.Bound)
	_, _ = fmt.Fprintf(out, "legacy: %d\n", s.Legacy)
}

func printSecretRebindResultHuman(out io.Writer, r secretRebindResult) {
	_, _ = fmt.Fprintf(out, "scanned:       %d\n", r.Scanned)
	_, _ = fmt.Fprintf(out, "rebound:       %d\n", r.Rebound)
	_, _ = fmt.Fprintf(out, "already_bound: %d\n", r.AlreadyBound)
	_, _ = fmt.Fprintf(out, "changed:       %d\n", r.Changed)
	_, _ = fmt.Fprintf(out, "failed:        %d\n", r.FailedCount)
	_, _ = fmt.Fprintf(out, "remaining:     %d\n", r.Remaining)
	for _, f := range r.Failed {
		_, _ = fmt.Fprintf(out, "  %s/%s: %s\n", f.Owner, f.Key, f.Reason)
	}
}
