package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// One compose file creates one app plus its member services in a single call.
func runAppsDeployCompose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploy-compose", "print the full deploy result as JSON to stdout and nothing else", stderr)
	var file string
	fs.StringVar(&file, "file", "", "path to a Docker Compose YAML file (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeployComposeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploy-compose requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--file is required"))
	}

	data, readErr := os.ReadFile(file) //nolint:gosec // operator-supplied CLI flag, the same pattern apps_create.go's own --file read uses
	if readErr != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read %s: %w", file, readErr))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.DeployCompose(context.Background(), name, data)
	if err != nil {
		printComposeFileWarnings(stderr, file, data)
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy compose to app %q: %w", name, err))
	}
	// The API's notices win; the local parse only covers an older server.
	if len(result.Notices) > 0 {
		for _, n := range result.Notices {
			if n.Level == "warning" {
				_, _ = fmt.Fprintf(stderr, "warning: %s: %s\n", file, n.Message)
			}
		}
	} else {
		printComposeFileWarnings(stderr, file, data)
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printComposeDeployResultHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// printComposeFileWarnings names every compose key the deploy will ignore, so
// nothing is dropped silently. A file that does not parse is left to the API error.
func printComposeFileWarnings(stderr io.Writer, path string, data []byte) {
	f, err := compose.Parse(data)
	if err != nil {
		return
	}
	for _, n := range f.Notices() {
		if n.Key != "" {
			_, _ = fmt.Fprintf(stderr, "warning: %s: %s\n", path, n.Message)
		}
	}
}

// printComposeDeployResultHuman prints "apps deploy-compose" output: the
// app_id plus one line per deployed service. Not printAppHuman, which is
// shaped for exactly one app.
func printComposeDeployResultHuman(out io.Writer, r composeDeployResult) {
	_, _ = fmt.Fprintf(out, "app_id: %s\n", r.AppID)
	// Services are recorded here; their containers start afterwards, so do not claim they run yet.
	_, _ = fmt.Fprintf(out, "accepted %d service(s), containers are starting:\n", len(r.Services))
	for _, svc := range r.Services {
		_, _ = fmt.Fprintf(out, "  %s\t%s\n", svc.Name, svc.Image)
	}
	_, _ = fmt.Fprintln(out, "app_id is the group; pass a service name above to apps get, logs or deploys")
}

func appsDeployComposeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy-compose <name> --file compose.yaml [flags]

Parses a Docker Compose YAML file and deploys it as app <name>, one
member service per compose service.

Flags:
  --file string           path to a Docker Compose YAML file (required)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the full deploy result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
