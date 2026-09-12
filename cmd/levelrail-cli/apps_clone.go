package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsClone implements "apps clone <name> <new-name>": POST
// /api/v1/apps/{name}/clone. Domains, secret values, and node placement
// never carry over to the clone (internal/api/apps_clone.go's own doc
// comment); the caller re-sets those explicitly afterward.
func runAppsClone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps clone", "print the cloned app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsCloneUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps clone", "an app name and a new name", 2)
	if !ok {
		return exitUsage
	}
	name, newName := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	cloned, err := client.CloneApp(context.Background(), name, newName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clone app %q to %q: %w", name, newName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, cloned, func() { printAppHuman(stdout, cloned) })
}

func appsCloneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps clone <name> <new-name> [flags]

Duplicates <name>'s desired state (image, port, env, secret names,
resources, health checks, strategy, replicas, project) under <new-name>.
Domains never carry over, since the source app still owns them; secret
values never carry over, only their names, so the clone starts with
those secrets unset; the clone always starts on the local node
regardless of where <name> is pinned. Fails with a conflict if
<new-name> already exists.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the cloned app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
