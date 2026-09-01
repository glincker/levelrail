package main

import (
	"context"
	"fmt"
	"io"
)

// deployOrRollbackConfig holds the wording that differs between "apps
// deploy" and "apps rollback", which otherwise send the identical
// request (client.DeployApp) and share every other line of logic; see
// runAppsDeploy's own doc comment for why there are two commands for
// one underlying mechanism at all.
type deployOrRollbackConfig struct {
	cmdLabel      string
	imageHelp     string
	confirmHelp   string
	usage         func(string) string
	errContext    string
	successFormat string
}

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
// same mutation). runAppsDeployOrRollback below is their shared
// implementation, parameterized by only the wording that differs.
//
// If name is tagged with a protected environment, the server rejects an
// unconfirmed deploy with a 409; --confirm skips straight past that,
// and omitting it falls back to an interactive "yes" prompt on stdin
// (resolveProtectedEnvironmentConfirmation), the same interactive-vs-
// scripted shape backups_restore.go's own resolveRestoreConfirmation
// uses for a destructive database restore.
func runAppsDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	return runAppsDeployOrRollback(prog, args, stdout, stderr, lookupEnv, stdin, deployOrRollbackConfig{
		cmdLabel:      "apps deploy",
		imageHelp:     "image reference to deploy, e.g. registry.example.com/org/app:tag (required)",
		confirmHelp:   "confirm deploying into a protected environment; omit to be prompted interactively if needed",
		usage:         appsDeployUsage,
		errContext:    "deploy",
		successFormat: "app %q now targets image %q; reconcile is asynchronous, check \"%s apps status %s\"\n",
	})
}

// runAppsDeployOrRollback is runAppsDeploy/runAppsRollback's shared
// implementation: see runAppsDeploy's own doc comment for why there are
// two commands over one underlying mechanism.
func runAppsDeployOrRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader, cfg deployOrRollbackConfig) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, cfg.cmdLabel, "print the updated app as JSON to stdout and nothing else", stderr)
	var image string
	var confirm bool
	fs.StringVar(&image, "image", "", cfg.imageHelp)
	fs.BoolVar(&confirm, "confirm", false, cfg.confirmHelp)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cfg.usage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires exactly one app name\n\n", prog, cfg.cmdLabel)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if image == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--image is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	updated, err := confirmProtectedEnvironment(confirm, stdin, stderr, func(confirm bool) (appResource, error) {
		return client.DeployApp(ctx, name, image, confirm)
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s app %q: %w", cfg.errContext, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stderr, cfg.successFormat, updated.Name, updated.Image, prog, updated.Name)
		printAppHuman(stdout, updated)
	})
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
