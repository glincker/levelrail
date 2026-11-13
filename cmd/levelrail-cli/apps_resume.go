package main

import (
	"context"
	"fmt"
	"io"
)

// Lets an app the crash-loop breaker quarantined start again.
func runAppsResume(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps resume", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsResumeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps resume requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	n, err := client.ResumeApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("resume app %q: %w", name, err))
	}
	result := map[string]any{"app": name, "resumed": n}
	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		if n == 0 {
			_, _ = fmt.Fprintf(stdout, "app %q has no quarantined containers\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "app %q resumed, %d container(s) released from quarantine\n", name, n)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsResumeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps resume <name> [flags]

Releases an app whose containers the crash-loop breaker quarantined after
repeated restarts, so the reconciler starts them again. Fix the cause first
(see "apps logs"), or it will be quarantined again. "apps restart" also
releases the app.

Flags:
  --token string     API token (default: %[2]s env var, then the credentials file)
  --api-url string   control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string   named credentials profile to read
  --json             print the result as JSON to stdout, nothing else
  -h, --help         show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
