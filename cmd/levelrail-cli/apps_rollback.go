package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsRollback implements "apps rollback <name> --image <ref>". There
// is no dedicated rollback endpoint or wire shape server-side: this sends
// the exact same POST /api/v1/apps/{name}/deploys request "apps deploy"
// does (client.go's own DeployApp, internal/api/deploys.go's
// handleTriggerDeploy), an older, already-known image tag standing in
// for a newer one, per that handler's own doc comment framing a redeploy
// to an older tag as identical to one to a newer tag.
//
// This mirrors how the web frontend already treats rollback:
// web/src/components/DeployAttemptsList.tsx's "Rollback to this build"
// button calls useTriggerDeploy, the same mutation its "existing image"
// deploy form uses, and only changes the toast copy ("Rollback
// triggered" / "Rolling back...") around it. This command does the same
// thing at the CLI layer: a thin, discoverability wrapper around
// DeployApp with rollback-framed usage text and diagnostics, not a
// second code path, so there is exactly one place ("apps deploy") that
// could ever drift from what the server actually does with this
// request.
//
// Flag shape matches "apps deploy" exactly (--image, not a second
// positional argument): a caller who already knows that command's shape
// gets an identical one here, and there is no server-side image-history
// endpoint this CLI could use to resolve "the previous tag" on its own,
// so the caller must already know which tag to roll back to either way
// (see apps_deploy.go's own doc comment on that same gap).
//
// --confirm/interactive-prompt behavior on a protected environment
// mirrors apps_deploy.go's own doc comment exactly: same server-side
// gate, same client-side fallback.
func runAppsRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" apps rollback", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, image string
	var jsonOut, confirm bool
	fs.StringVar(&image, "image", "", "older, already-built image reference to roll back to, e.g. registry.example.com/org/app:tag (required)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print the updated app as JSON to stdout and nothing else")
	fs.BoolVar(&confirm, "confirm", false, "confirm rolling back into a protected environment; omit to be prompted interactively if needed")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRollbackUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps rollback requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	if image == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--image is required"))
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
	ctx := context.Background()

	updated, err := client.DeployApp(ctx, name, image, confirm)
	if !confirm {
		if apiErr, ok := protectedEnvironmentError(err); ok {
			confirmed, cerr := resolveProtectedEnvironmentConfirmation(apiErr.Message, stdin, stderr)
			if cerr != nil {
				return reportError(stdout, stderr, jsonOut, cerr)
			}
			if confirmed {
				updated, err = client.DeployApp(ctx, name, image, true)
			}
		}
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("roll back app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "app %q rolling back to image %q; reconcile is asynchronous, check \"%s apps status %s\"\n", updated.Name, updated.Image, prog, updated.Name)
	printAppHuman(stdout, updated)
	return exitOK
}

func appsRollbackUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps rollback <name> --image IMAGE [flags]

Points an existing app's desired image back at an older, already-built
IMAGE and returns immediately; the next reconcile converges the running
container to it. This is the same request "%[1]s apps deploy <name>
--image IMAGE" sends (there is no separate rollback endpoint), given as
its own command for a caller who wants to say "rollback" rather than
"deploy" and get a rollback-framed message back. There is no image
history lookup here: pass the exact older tag you want to roll back to
("%[1]s apps get <name> --json" or "%[1]s apps logs <name>" can help you
find it).

This command does not validate that IMAGE exists or ever ran
successfully before returning: exit 0 only means the server accepted
the request, not that the rollback converged. A typo'd or never-built
tag, or the app's already-current image, both report the same success
here; check "%[1]s apps status <name>" to confirm the rollback
actually landed before treating this as done.

If the app is tagged with a protected environment, this fails unless
--confirm is set or you type "yes" at the interactive prompt.

Flags:
  --image string          older, already-built image reference to roll back to (required)
  --confirm                  confirm rolling back into a protected environment, skipping the interactive prompt
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the updated app as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
