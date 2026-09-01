package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runVersion implements "version": GET /api/v1/updates, the same
// endpoint routes/settings/updates.tsx already reads from, reporting the
// control plane's running version against GitHub's latest published
// release.
func runVersion(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "version", "print version info as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, versionUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	updates, err := client.GetUpdates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get updates: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updates); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printUpdatesHuman(stdout, updates)
	return exitOK
}

func versionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s version [flags]

Shows the control plane's running version, and whether a newer release
has been published to github.com/glincker/levelrail.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print version info as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
