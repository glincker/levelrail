package main

import (
	"context"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"io"
)

// Stop only sets a suspended flag; the reconciler stops the container on its next pass.
func runAppsStop(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps stop", "print the app as JSON to stdout and nothing else", stderr)
	var queueOffline bool
	fs.BoolVar(&queueOffline, "queue-while-offline", false, "if the app's node is offline, record the change as pending and apply it when the node returns (default: refuse)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsStopUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps stop", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	stopped, err := appAction(context.Background(), client, name, "stop", queueOffline, func() (apiclient.AppResource, error) { return client.StopApp(context.Background(), name) })
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("stop app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, stopped, func() {
		_, _ = fmt.Fprintf(stderr, "app %q stop requested; reconcile is asynchronous, check \"%s apps status %s\"\n", stopped.Name, prog, stopped.Name)
		printAppHuman(stdout, stopped)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsStopUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps stop <name> [flags]

Stops an app's running container without changing its desired state.
Reversed with "%[1]s apps start <name>".

Flags:
  --queue-while-offline   if the app's node is offline, record the change as pending (default: refuse)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
