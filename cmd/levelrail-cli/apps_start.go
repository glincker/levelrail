package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsStart implements "apps start <name>": POST
// /api/v1/apps/{name}/start (internal/api/apps.go's own handleStartApp),
// AbilityDeploy-gated, no request body. Clears the suspended flag "apps
// stop" set, letting the reconciler bring the container back.
func runAppsStart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps start", "print the app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsStartUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps start", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.StartApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, started, func() {
		_, _ = fmt.Fprintf(stderr, "app %q start requested; reconcile is asynchronous, check \"%s apps status %s\"\n", started.Name, prog, started.Name)
		printAppHuman(stdout, started)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsStartUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps start <name> [flags]

Starts an app previously stopped with "%[1]s apps stop <name>".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
