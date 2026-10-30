package main

import (
	"context"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"io"
)

// Clears the suspended flag set by apps stop, so the reconciler restarts the container.
func runAppsStart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps start", "print the app as JSON to stdout and nothing else", stderr)
	var queueOffline bool
	fs.BoolVar(&queueOffline, "queue-while-offline", false, "if the app's node is offline, record the change as pending and apply it when the node returns (default: refuse)")
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

	started, err := appAction(context.Background(), client, name, "start", queueOffline, func() (apiclient.AppResource, error) { return client.StartApp(context.Background(), name) })
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
