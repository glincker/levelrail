package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
)

// runAppsPreviews dispatches "apps previews <verb> [flags]" to one of
// list/teardown/enable/disable, the CLI counterpart of
// internal/api/preview_environments_handlers.go's own routes: preview
// environments per pull request, opt-in per app.
func runAppsPreviews(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsPreviewsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsPreviewsUsage(prog))
		return exitOK
	case "list":
		return runAppsPreviewsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "teardown":
		return runAppsPreviewsTeardown(prog, args[1:], stdout, stderr, lookupEnv)
	case "enable":
		return runAppsPreviewsSetEnabled(prog, args[1:], stdout, stderr, lookupEnv, true)
	case "disable":
		return runAppsPreviewsSetEnabled(prog, args[1:], stdout, stderr, lookupEnv, false)
	case "sweep":
		return runAppsPreviewsSweep(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps previews subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsPreviewsUsage(prog))
		return exitUsage
	}
}

func appsPreviewsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews list <app-name> [flags]                list active previews for an app
  %[1]s apps previews teardown <app-name> <pr-number> [flags]   tear down one PR's preview right now
  %[1]s apps previews enable <app-name> [flags]              opt an app into preview environments
  %[1]s apps previews disable <app-name> [flags]             opt an app back out
  %[1]s apps previews sweep [flags]                          tear down every stale preview now, across all apps

A preview environment deploys automatically when a pull request opens
or gets new commits against a git-connected app with previews enabled,
and tears down automatically when that pull request closes or merges.
"teardown" is the manual safety net for a stuck build or a retry after
a partially-failed automatic teardown. "sweep" is the manual trigger for
the TTL fallback that runs automatically in the background: any preview
whose pull-request-closed webhook never arrived (a failed delivery, see
"apps webhook-deliveries") is torn down once it goes stale, "list"'s own
STALE column shows which ones a sweep would catch right now.

Run "%[1]s apps previews <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsPreviewsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps previews list", "print previews as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsListUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	appName, ok := requireOneArg(fs, stderr, prog, "apps previews list", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	previews, err := client.ListPreviewEnvironments(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list previews for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, previews, func() { printPreviewEnvironmentsTable(stdout, previews) })
}

func printPreviewEnvironmentsTable(out io.Writer, previews []previewEnvironmentResource) {
	if len(previews) == 0 {
		_, _ = fmt.Fprintln(out, "no active preview environments")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PR\tPREVIEW APP\tBRANCH\tSTATUS\tDOMAIN\tUPDATED\tSTALE")
	for _, p := range previews {
		domain := p.Domain
		if domain == "" {
			domain = "-"
		}
		stale := "no"
		if p.Stale {
			stale = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", p.PRNumber, p.PreviewAppID, p.Branch, p.Status, domain, p.UpdatedAt, stale)
	}
	_ = tw.Flush()
}

func appsPreviewsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews list <app-name> [flags]

Lists an app's active pull-request preview environments.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print previews as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsPreviewsTeardown(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps previews teardown", "print {} to stdout on success instead of a plain confirmation", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsTeardownUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	positional := fs.Args()
	if len(positional) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps previews teardown requires an app name and a pr number\n\n", prog)
		_, _ = fmt.Fprint(stderr, appsPreviewsTeardownUsage(prog))
		return exitUsage
	}
	appName := positional[0]
	prNumber, err := strconv.Atoi(positional[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("pr number must be an integer"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.TeardownPreviewEnvironment(context.Background(), appName, prNumber); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("teardown preview for app %q pr #%d: %w", appName, prNumber, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, map[string]any{}, func() {
		_, _ = fmt.Fprintf(stdout, "preview for app %q pr #%d torn down\n", appName, prNumber)
	})
}

func appsPreviewsTeardownUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews teardown <app-name> <pr-number> [flags]

Tears down one pull request's preview environment right now: its
service(s), its app grouping, and its own record. Idempotent, same as
the automatic pull-request-closed teardown.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {} to stdout on success instead of a plain confirmation
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsPreviewsSetEnabled(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), enabled bool) int {
	verb := "enable"
	if !enabled {
		verb = "disable"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps previews "+verb, "print the resulting setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsSetEnabledUsage(prog, verb)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	appName, ok := requireOneArg(fs, stderr, prog, "apps previews "+verb, "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.SetPreviewEnabled(context.Background(), appName, enabled)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s previews for app %q: %w", verb, appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, result, func() {
		_, _ = fmt.Fprintf(stdout, "preview environments %sd for app %q\n", verb, appName)
	})
}

func appsPreviewsSetEnabledUsage(prog, verb string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews %[5]s <app-name> [flags]

Requires a connected git source (%[1]s does not expose connecting one
yet; use the dashboard's Git source card first).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the resulting setting as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, verb)
}

func runAppsPreviewsSweep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps previews sweep", "print the swept count as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsSweepUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.SweepPreviewEnvironments(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("sweep preview environments: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, result, func() {
		_, _ = fmt.Fprintf(stdout, "swept %d stale preview environment(s)\n", result.Swept)
	})
}

func appsPreviewsSweepUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews sweep [flags]

Tears down every preview environment, across every app, whose own
pull-request-closed webhook never arrived: the manual trigger for the
same TTL fallback that already runs automatically in the background.
Idempotent, same teardown path "teardown" and the automatic
pull-request-closed webhook both use.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the swept count as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
