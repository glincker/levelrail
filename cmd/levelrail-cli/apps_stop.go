package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsStop implements "apps stop <name>": POST /api/v1/apps/{name}/stop
// (internal/api/apps.go's own handleStopApp), AbilityDeploy-gated, no
// request body. Marks the service suspended so the reconciler stops its
// container on the next pass; "apps start" clears the flag again.
func runAppsStop(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps stop", "print the app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsStopUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	name, ok := requireOneArg(fs, stderr, prog, "apps stop", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	stopped, err := client.StopApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("stop app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, stopped); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "app %q stop requested; reconcile is asynchronous, check \"%s apps status %s\"\n", stopped.Name, prog, stopped.Name)
	printAppHuman(stdout, stopped)
	return exitOK
}

func appsStopUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps stop <name> [flags]

Stops an app's running container without changing its desired state.
Reversed with "%[1]s apps start <name>".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the app as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
