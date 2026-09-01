package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsDeploy implements "apps deploy <name> --image <ref>": POST
// /api/v1/apps/{name}/deploys (internal/api/deploys.go's own
// handleTriggerDeploy). Points the app's desired image at a new tag and
// returns as soon as that's saved; the application controller's next
// reconcile is what actually converges a running container to it, so
// this command does not wait for that to finish, matching the server's
// own 202 Accepted response. Use "apps status" to watch the reconcile.
//
// There is no dedicated rollback endpoint: handleTriggerDeploy's own doc
// comment documents rollback as exactly this same mechanism run with an
// older tag ("pointing desired.Image back at an older tag and
// reconciling converges to it the same way any other redeploy does"),
// and there is no image-history endpoint this CLI could use to look up
// "the previous tag" on the caller's behalf, so a caller who wants to
// roll back must already know the tag either way. "apps rollback"
// (apps_rollback.go) exists anyway, as a thin, discoverability-only
// wrapper around this same command for a caller who wants to type
// "rollback" and get rollback-framed output, the same distinction the
// web frontend's DeployAttemptsList.tsx already draws between its
// deploy form and its "Rollback to this build" button (both call the
// same mutation).
//
// If name is tagged with a protected environment, the server rejects an
// unconfirmed deploy with a 409; --confirm skips straight past that,
// and omitting it falls back to an interactive "yes" prompt on stdin
// (resolveProtectedEnvironmentConfirmation), the same interactive-vs-
// scripted shape backups_restore.go's own resolveRestoreConfirmation
// uses for a destructive database restore.
func runAppsDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" apps deploy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, image string
	var jsonOut, confirm bool
	fs.StringVar(&image, "image", "", "image reference to deploy, e.g. registry.example.com/org/app:tag (required)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	var profileFlag string
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print the updated app as JSON to stdout and nothing else")
	fs.BoolVar(&confirm, "confirm", false, "confirm deploying into a protected environment; omit to be prompted interactively if needed")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeployUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploy requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if image == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--image is required"))
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))
	ctx := context.Background()

	updated, err := confirmProtectedEnvironment(confirm, stdin, stderr, func(confirm bool) (appResource, error) {
		return client.DeployApp(ctx, name, image, confirm)
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "app %q now targets image %q; reconcile is asynchronous, check \"%s apps status %s\"\n", updated.Name, updated.Image, prog, updated.Name)
	printAppHuman(stdout, updated)
	return exitOK
}

func appsDeployUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy <name> --image IMAGE [flags]

Points an existing app's desired image at IMAGE and returns immediately;
the next reconcile converges the running container to it. Redeploying an
older, already-known image tag with this same command is what a
rollback is (there is no separate rollback endpoint); "%[1]s apps
rollback <name> --image <older-tag>" does exactly this, framed for that
use.

If the app is tagged with a protected environment, this fails unless
--confirm is set or you type "yes" at the interactive prompt.

Flags:
  --image string          image reference to deploy, e.g. registry.example.com/org/app:tag (required)
  --confirm                  confirm deploying into a protected environment, skipping the interactive prompt
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
