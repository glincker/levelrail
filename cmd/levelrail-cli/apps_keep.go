package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runAppsKeep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps keep", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsKeepUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps keep", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.KeepTrialApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("keep app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if res.AlreadyKept {
			_, _ = fmt.Fprintf(stdout, "app %q was already kept as a permanent app\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "app %q is now a permanent app (%s)\n", name, strings.Join(res.Kept, ", "))
		_, _ = fmt.Fprintf(stdout, "next: add a domain, a volume backup schedule and an alert rule, see \"%s apps overview %s\"\n", prog, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsKeepUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps keep <name> [flags]

Keeps a one-click trial deployment as a permanent app: it stops being
flagged as throwaway, with every service of the same app. Repeating it is
harmless; an app that never was a trial is refused.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
