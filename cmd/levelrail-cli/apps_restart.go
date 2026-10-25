package main

import (
	"context"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"io"
)

// Recreates the container without an image change, since a plain redeploy of the same tag is a no-op.
func runAppsRestart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps restart", "print the app as JSON to stdout and nothing else", stderr)
	var queueOffline bool
	fs.BoolVar(&queueOffline, "queue-while-offline", false, "if the app's node is offline, record the change as pending and apply it when the node returns (default: refuse)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRestartUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps restart requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	restarted, err := appAction(context.Background(), client, name, "restart", queueOffline, func() (apiclient.AppResource, error) { return client.RestartApp(context.Background(), name) })
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restart app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, restarted, func() {
		_, _ = fmt.Fprintf(stderr, "app %q restart requested; rolling restart in progress, follow it with \"%s apps deploys wait %s\"\n", restarted.Name, prog, restarted.Name)
		printAppHuman(stdout, restarted)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsRestartUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps restart <name> [flags]

Forces an existing app's running container to be recreated with no image
change. Different from "apps deploy" and "apps rollback": neither of
those force a restart when the image is unchanged; this command always
does, useful when a process needs a fresh start without a new build.

The restart is rolling: a new container must pass readiness before traffic
switches, and the old one is kept for a while. It also picks up saved env
changes. It is recorded as a deploy with trigger "restart", so "apps deploys
list" and "apps deploys wait" cover it. A stopped app is refused: run
"apps start" first.

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
