package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsScale implements "apps scale <name> --replicas N [--strategy S]":
// a read-modify-write of the app's replicas and strategy through PUT
// /api/v1/apps/{name}, so a prebuilt-image app can be scaled without a spec file.
func runAppsScale(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps scale", "print the app as JSON to stdout and nothing else", stderr)
	var replicas int
	var strategy, memory string
	var cpu float64
	fs.IntVar(&replicas, "replicas", 0, "number of container replicas (at least 1; use \"apps stop\" to run none)")
	fs.StringVar(&strategy, "strategy", "", "deploy strategy: rolling, recreate, or blue-green (default: unchanged)")
	fs.StringVar(&memory, "memory", "", "memory limit, e.g. 512Mi or 2Gi (default: unchanged)")
	fs.Float64Var(&cpu, "cpu", 0, "CPU limit in cores, e.g. 0.5 (default: unchanged)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScaleUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps scale", "app name")
	if !ok {
		return exitUsage
	}
	if replicas < 0 || cpu < 0 || (replicas == 0 && strategy == "" && memory == "" && cpu == 0) {
		return reportError(stdout, stderr, jsonOut, newValidationError("apps scale requires --replicas N (N >= 1), --strategy, --memory and/or --cpu"))
	}
	memoryBytes, err := parseOptionalMemory(memory)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("--memory: %v", err))
	}
	if strategy != "" && strategy != "rolling" && strategy != "recreate" && strategy != "blue-green" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--strategy must be rolling, recreate, or blue-green, got %q", strategy))
	}

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}
	if replicas > 0 {
		app.Replicas = replicas
	}
	if strategy != "" {
		app.Strategy = strategy
	}
	if memoryBytes > 0 || cpu > 0 {
		res := serviceResources{}
		if app.Resources != nil {
			res = *app.Resources
		}
		if memoryBytes > 0 {
			res.MemoryBytes = memoryBytes
		}
		if cpu > 0 {
			res.NanoCPUs = int64(cpu * 1e9)
		}
		app.Resources = &res
	}
	updated, err := client.UpdateApp(ctx, name, app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("scale app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stderr, "app %q scaled to %d replica(s), strategy %s; reconcile is asynchronous, check \"%s apps status %s\"\n", updated.Name, updated.Replicas, updated.Strategy, prog, updated.Name)
		printAppHuman(stdout, updated)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func parseOptionalMemory(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return parseMemoryBytes(s)
}

func appsScaleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scale <name> [--replicas N] [--strategy rolling|recreate|blue-green] [--memory 512Mi] [--cpu 0.5] [flags]

Sets an app's replica count, deploy strategy and/or resource limits. Works
for any app, including one running a prebuilt image. Limits apply to a
running container live when Docker allows it, otherwise at the next restart.

Flags:
  --replicas int          number of container replicas (at least 1)
  --strategy string       deploy strategy: rolling, recreate, or blue-green
  --memory string         memory limit, e.g. 512Mi or 2Gi
  --cpu float             CPU limit in cores, e.g. 0.5
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
