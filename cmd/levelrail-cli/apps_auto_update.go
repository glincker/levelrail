package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// runAppsAutoUpdate dispatches "apps auto-update enable|disable|status|check":
// the CLI for GET/PUT /api/v1/apps/{name}/auto-update and its check route.
func runAppsAutoUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsAutoUpdateUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsAutoUpdateUsage(prog))
		return exitOK
	case "enable", "disable", "status", "check", "webhook":
		return runAppsAutoUpdateVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps auto-update subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsAutoUpdateUsage(prog))
		return exitUsage
	}
}

func appsAutoUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps auto-update enable <app-name> [flags]    redeploy automatically when the image tag moves
  %[1]s apps auto-update disable <app-name> [flags]   opt back out
  %[1]s apps auto-update status <app-name> [flags]    show the setting and the last check
  %[1]s apps auto-update check <app-name> [flags]     check the registry now, redeploying if it moved
  %[1]s apps auto-update webhook <app-name> [flags]   mint a registry push webhook URL (shown once)

Point your registry's push webhook (Docker Hub, Harbor, any POST) at the
webhook URL to update on push instead of waiting for the interval.

Off by default. Enabled apps are checked on the control plane's interval
(APP_IMAGE_UPDATE_INTERVAL, default 1h). Apps built from source, stopped,
frozen, or in a protected environment are skipped, never forced.
`, prog)
}

func runAppsAutoUpdateVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps auto-update "+verb, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps auto-update %s <app-name> [flags]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps auto-update "+verb, "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if verb == "webhook" {
		hook, err := client.RotateImageUpdateWebhook(ctx, appName)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("auto-update webhook for app %q: %w", appName, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, hook, func() {
			_, _ = fmt.Fprintf(stdout, "webhook URL (shown once, replaces any earlier one):\n  POST %s%s\n", strings.TrimRight(resolveAPIURL(apiURLFlag, lookupEnv, prog, resolveProfile(profileFlag, lookupEnv)), "/"), hook.Path)
		})
	}
	var (
		result imageAutoUpdateResource
		err    error
	)
	switch verb {
	case "enable":
		result, err = client.SetImageAutoUpdate(ctx, appName, true)
	case "disable":
		result, err = client.SetImageAutoUpdate(ctx, appName, false)
	case "check":
		result, err = client.CheckImageAutoUpdate(ctx, appName)
	default:
		result, err = client.GetImageAutoUpdate(ctx, appName)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("auto-update %s for app %q: %w", verb, appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printImageAutoUpdate(stdout, result) })
}

func printImageAutoUpdate(out io.Writer, r imageAutoUpdateResource) {
	_, _ = fmt.Fprintf(out, "auto-update: %t\n", r.Enabled)
	_, _ = fmt.Fprintf(out, "webhook:     %t\n", r.HasWebhook)
	if r.LastCheckedAt != nil {
		_, _ = fmt.Fprintf(out, "last check:  %s\n", r.LastCheckedAt.Format(time.RFC3339))
	}
	if r.LastResult != "" {
		_, _ = fmt.Fprintf(out, "result:      %s\n", r.LastResult)
	}
}
