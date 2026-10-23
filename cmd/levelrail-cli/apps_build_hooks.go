package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// runAppsBuildHooks dispatches "apps build-hooks list|create|rotate|revoke": the CLI for /api/v1/apps/{name}/build-hooks.
func runAppsBuildHooks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBuildHooksUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBuildHooksUsage(prog))
		return exitOK
	case "list", "create", "rotate", "revoke":
		return runAppsBuildHooksVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps build-hooks subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsBuildHooksUsage(prog))
		return exitUsage
	}
}

func appsBuildHooksUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps build-hooks list <app-name> [flags]                  list the app's hooks and when each last fired
  %[1]s apps build-hooks create <app-name> --name NAME [--branch B] [flags]   mint a hook URL (shown once)
  %[1]s apps build-hooks rotate <app-name> <hook-id> [flags]      replace a hook's secret, the old URL stops working
  %[1]s apps build-hooks revoke <app-name> <hook-id> [flags]      delete a hook

POST to the hook URL (no auth header) to build and deploy the app's git source.
An optional JSON body {"title": "...", "body": "..."} reaches the build as the
TRIGGER_TITLE and TRIGGER_BODY build args (declare them with ARG).
`, prog)
}

func runAppsBuildHooksVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps build-hooks " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	hookName := fs.String("name", "", "hook name (create)")
	branch := fs.String("branch", "", "branch to build, default the app's git branch (create)")
	wantArgs, argsLabel, usageArgs := 1, "an app name", "<app-name>"
	if verb == "rotate" || verb == "revoke" {
		wantArgs, argsLabel, usageArgs = 2, "an app name and a hook id", "<app-name> <hook-id>"
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s %s [flags]\n\n", prog, label, usageArgs)
		_, _ = fmt.Fprint(stderr, appsBuildHooksUsage(prog))
		_, _ = fmt.Fprint(stderr, "\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, label, argsLabel, wantArgs)
	if !ok {
		return exitUsage
	}
	if verb == "create" && strings.TrimSpace(*hookName) == "" {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires --name\n", prog, label)
		return exitUsage
	}
	appName := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	baseURL := strings.TrimRight(resolveAPIURL(apiURLFlag, lookupEnv, prog, resolveProfile(profileFlag, lookupEnv)), "/")

	var (
		result any
		plain  func()
		err    error
	)
	switch verb {
	case "list":
		var hooks []buildHookResource
		hooks, err = client.ListBuildHooks(ctx, appName)
		result, plain = hooks, func() { printBuildHooks(stdout, hooks) }
	case "create", "rotate":
		var h buildHookResource
		if verb == "create" {
			h, err = client.CreateBuildHook(ctx, appName, strings.TrimSpace(*hookName), strings.TrimSpace(*branch))
		} else {
			h, err = client.RotateBuildHook(ctx, appName, rest[1])
		}
		result, plain = h, func() {
			_, _ = fmt.Fprintf(stdout, "hook %s (%s)\nURL (shown once):\n  POST %s%s\n", h.Name, h.ID, baseURL, h.Path)
		}
	default:
		if err = client.RevokeBuildHook(ctx, appName, rest[1]); err == nil {
			result, plain = map[string]string{"revoked": rest[1]}, func() { _, _ = fmt.Fprintf(stdout, "revoked %s\n", rest[1]) }
		}
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("build-hooks %s for app %q: %w", verb, appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, plain)
}

func printBuildHooks(out io.Writer, hooks []buildHookResource) {
	if len(hooks) == 0 {
		_, _ = fmt.Fprintln(out, "no build hooks")
		return
	}
	for _, h := range hooks {
		last, branch := "never", "default"
		if h.LastTriggeredAt != nil {
			last = h.LastTriggeredAt.Format(time.RFC3339)
		}
		if h.Branch != "" {
			branch = h.Branch
		}
		_, _ = fmt.Fprintf(out, "%s  %s  branch=%s  runs=%d  last=%s  %s\n", h.ID, h.Name, branch, h.TriggerCount, last, h.LastResult)
	}
}
