package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsRestart implements "apps restart <name>": POST
// /api/v1/apps/{name}/restart (internal/api/apps.go's own
// handleRestartApp), AbilityDeploy-gated, no request body. Forces a
// running container to be recreated without an image change (a plain
// redeploy of the same image tag is otherwise a genuine reconciler
// no-op, per handleRestartApp's own doc comment), which is useful on its
// own merit (an app that needs a fresh process without a new image, a
// stuck container the reconciler hasn't noticed yet) and is a distinct
// operation from both "apps deploy" and "apps rollback": neither of
// those change the image, this command never does.
func runAppsRestart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps restart", "print the app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRestartUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps restart requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	restarted, err := client.RestartApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restart app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, restarted); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "app %q restart requested; reconcile is asynchronous, check \"%s apps status %s\"\n", restarted.Name, prog, restarted.Name)
	printAppHuman(stdout, restarted)
	return exitOK
}

func appsRestartUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps restart <name> [flags]

Forces an existing app's running container to be recreated with no image
change. Different from "apps deploy" and "apps rollback": neither of
those force a restart when the image is unchanged; this command always
does, useful when a process needs a fresh start without a new build.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the app as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
