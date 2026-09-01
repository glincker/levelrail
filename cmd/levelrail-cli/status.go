package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runStatus implements "status": GET /api/v1/system/status, the same
// endpoint routes/settings/general.tsx's Docker/disk cards already read
// from and the dashboard-wide health banner polls. Top-level rather than
// under "nodes", since this reports the control plane's own local Docker
// reachability, not a per-node signal (see internal/apiclient's
// SystemStatusResource doc comment).
func runStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "status", "print status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, statusUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	status, err := client.GetSystemStatus(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get system status: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, status); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printSystemStatusHuman(stdout, status)
	return exitOK
}

func statusUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s status [flags]

Shows this control plane's own configured/not-configured signals,
including whether its local Docker daemon is currently reachable.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print status as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
