package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsScale implements "apps scale <name> --replicas N [--strategy S]":
// a read-modify-write of the app's replicas and strategy through PUT
// /api/v1/apps/{name}, so a prebuilt-image app can be scaled without a spec file.
func runAppsScale(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps scale", "print the app as JSON to stdout and nothing else", stderr)
	var replicas int
	var strategy, memory, swap, cpuset string
	var cpu float64
	fs.IntVar(&replicas, "replicas", 0, "number of container replicas (at least 1; use \"apps stop\" to run none)")
	fs.StringVar(&strategy, "strategy", "", "deploy strategy: rolling, recreate, or blue-green (default: unchanged)")
	fs.StringVar(&memory, "memory", "", "memory limit, e.g. 512Mi or 2Gi (default: unchanged)")
	fs.Float64Var(&cpu, "cpu", 0, "CPU limit in cores, e.g. 0.5 (default: unchanged)")
	fs.StringVar(&swap, "swap-memory", "", "memory plus swap limit, e.g. 1Gi; at least the memory limit (default: unchanged)")
	fs.StringVar(&cpuset, "cpuset-cpus", "", "CPUs the app may use, e.g. 0-1 or 0,2 (default: unchanged)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScaleUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: apps scale requires an app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	if len(rest) > 1 {
		if replicas != 0 || strategy != "" || memory != "" || cpu != 0 || swap != "" || cpuset != "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("service=count pairs cannot be combined with --replicas, --strategy, --memory, --cpu, --swap-memory or --cpuset-cpus"))
		}
		return scaleServices(prog, name, rest[1:], apiURLFlag, tokenFlag, profileFlag, jsonOut, of, stdout, stderr, lookupEnv)
	}
	if replicas < 0 || cpu < 0 || (replicas == 0 && strategy == "" && memory == "" && cpu == 0 && swap == "" && cpuset == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("apps scale requires --replicas N (N >= 1), --strategy, --memory, --cpu, --swap-memory and/or --cpuset-cpus"))
	}
	memoryBytes, err := parseOptionalMemory(memory)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("--memory: %v", err))
	}
	swapBytes, err := parseOptionalMemory(swap)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("--swap-memory: %v", err))
	}
	if cpuset != "" && !cpusetPattern.MatchString(cpuset) {
		return reportError(stdout, stderr, jsonOut, newValidationError("--cpuset-cpus must be a CPU list such as 0-1 or 0,2, got %q", cpuset))
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
	if memoryBytes > 0 || cpu > 0 || swapBytes > 0 || cpuset != "" {
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
		if swapBytes > 0 {
			// Docker's swap value is memory plus swap combined, so it cannot sit below the effective memory limit.
			if swapBytes < res.MemoryBytes {
				return reportError(stdout, stderr, jsonOut, newValidationError("--swap-memory (%s) must be at least the memory limit: it is memory+swap combined, not swap alone", swap))
			}
			res.SwapMemoryBytes = swapBytes
		}
		if cpuset != "" {
			res.CPUSetCPUs = cpuset
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

// parseScalePairs turns "web=2" arguments into ordered service/count pairs, rejecting every malformed one up front.
func parseScalePairs(args []string) ([]scalePair, error) {
	seen := map[string]bool{}
	var out []scalePair
	for _, a := range args {
		key, val, found := strings.Cut(a, "=")
		if !found || key == "" || val == "" {
			return nil, newValidationError("%q is not service=count, e.g. web=2 worker=1", a)
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			return nil, newValidationError("%q: the count must be a whole number of at least 1 (use \"apps stop\" to run none)", a)
		}
		if seen[key] {
			return nil, newValidationError("service %q is given twice", key)
		}
		seen[key] = true
		out = append(out, scalePair{service: key, replicas: n})
	}
	return out, nil
}

type scalePair struct {
	service  string
	replicas int
}

// scaleServices resolves each service key through the app group, then sets every replica count.
// Keys are checked first; a failed update then rolls back the services already changed.
func scaleServices(prog, app string, pairArgs []string, apiURLFlag, tokenFlag, profileFlag string, jsonOut bool, of outputFlags, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	pairs, err := parseScalePairs(pairArgs)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	group, err := client.GetAppGroup(ctx, app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get services of app %q: %w", app, err))
	}
	known := map[string]bool{}
	var keys []string
	for _, s := range group.Services {
		if key, ok := strings.CutPrefix(s.Name, app+"-"); ok {
			known[s.Name] = true
			keys = append(keys, key)
		}
	}
	for _, p := range pairs {
		if !known[serviceAppName(app, p.service)] {
			if len(keys) == 0 {
				return reportError(stdout, stderr, jsonOut, newValidationError("app %q has no named services; use --replicas N", app))
			}
			return reportError(stdout, stderr, jsonOut, newValidationError("app %q has no service %q (services: %s)", app, p.service, strings.Join(keys, ", ")))
		}
	}
	updated := make([]appResource, 0, len(pairs))
	previous := make([]appResource, 0, len(pairs))
	for _, p := range pairs {
		target := serviceAppName(app, p.service)
		res, err := client.GetApp(ctx, target)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, scaleFailure(ctx, client, stderr, previous, fmt.Errorf("get app %q: %w", target, err)))
		}
		before := res
		res.Replicas = p.replicas
		if res, err = client.UpdateApp(ctx, target, res); err != nil {
			return reportError(stdout, stderr, jsonOut, scaleFailure(ctx, client, stderr, previous, fmt.Errorf("scale service %q: %w", p.service, err)))
		}
		previous = append(previous, before)
		updated = append(updated, res)
	}
	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		for _, u := range updated {
			_, _ = fmt.Fprintf(stdout, "%s scaled to %d replica(s)\n", u.Name, u.Replicas)
		}
		_, _ = fmt.Fprintf(stderr, "reconcile is asynchronous, check \"%s apps status %s\"\n", prog, app)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// scaleFailure restores the replica counts of services already changed, best effort, and says what it did.
func scaleFailure(ctx context.Context, client *apiclient.Client, stderr io.Writer, applied []appResource, cause error) error {
	for i := len(applied) - 1; i >= 0; i-- {
		prev := applied[i]
		if _, err := client.UpdateApp(ctx, prev.Name, prev); err != nil {
			_, _ = fmt.Fprintf(stderr, "rollback of %q to %d replica(s) failed: %v\n", prev.Name, prev.Replicas, err)
			continue
		}
		_, _ = fmt.Fprintf(stderr, "rolled back %q to %d replica(s)\n", prev.Name, prev.Replicas)
	}
	return cause
}

// cpusetPattern accepts kernel CPU list syntax: numbers and ranges joined by commas.
var cpusetPattern = regexp.MustCompile(`^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$`)

func parseOptionalMemory(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return parseMemoryBytes(s)
}

func appsScaleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scale <name> [--replicas N] [--strategy rolling|recreate|blue-green] [--memory 512Mi] [--cpu 0.5] [--swap-memory 1Gi] [--cpuset-cpus 0-1] [flags]
  %[1]s apps scale <name> web=2 worker=1 [flags]

Sets an app's replica count, deploy strategy and/or resource limits. Works
for any app, including one running a prebuilt image. Limits apply to a
running container live when Docker allows it, otherwise at the next restart.

With service=count pairs, each named service of a multi-service app gets its
own replica count (counts of at least 1). Every key is checked before any
change. The updates then run one at a time, not atomically: if one fails, the
services already changed are set back to their previous counts (best effort,
each rollback is reported). Pairs cannot be combined with the flags below.

Flags:
  --replicas int          number of container replicas (at least 1)
  --strategy string       deploy strategy: rolling, recreate, or blue-green
  --memory string         memory limit, e.g. 512Mi or 2Gi
  --cpu float             CPU limit in cores, e.g. 0.5
  --swap-memory string    memory plus swap limit, e.g. 1Gi (at least the memory limit)
  --cpuset-cpus string    CPUs the app may use, e.g. 0-1 or 0,2
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
