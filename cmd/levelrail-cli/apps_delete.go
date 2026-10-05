package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsDelete implements "apps delete <name>": DELETE
// /api/v1/apps/{name}. A 202 means the app is deleted but its containers are
// still being removed in the background (retried until they are gone).
func runAppsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps delete", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	res, err := client.DeleteAppResult(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete app %q: %w", name, err))
	}

	payload := map[string]any{"deleted": true}
	if res.Pending {
		payload["teardown_pending"] = true
		payload["error"] = res.Error
	}
	if err := renderResult(stdout, of.Format, of.Query, payload, func() {
		if res.Pending {
			_, _ = fmt.Fprintf(stdout, "app %q deleted, but its containers are not removed yet (%s); teardown retries automatically\n", name, res.Error)
			return
		}
		_, _ = fmt.Fprintf(stdout, "app %q deleted, containers removed\n", name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps delete <name> [flags]

Removes an app and stops and removes its containers. If the node is
unreachable or a stop fails, the app is still deleted and teardown is
retried until it succeeds (the command then reports it as pending).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"deleted": true, "teardown_pending": bool} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
