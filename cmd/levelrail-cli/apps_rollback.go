package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// runAppsRollback implements "apps rollback <name> --image <ref>", the
// deploy request framed as a rollback and limited to known local tags.
func runAppsRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	return runAppsDeployOrRollback(prog, args, stdout, stderr, lookupEnv, stdin, deployOrRollbackConfig{
		cmdLabel:        "apps rollback",
		imageHelp:       "older, already-built image reference to roll back to, e.g. registry.example.com/org/app:tag (required)",
		confirmHelp:     "confirm rolling back into a protected environment; omit to be prompted interactively if needed",
		usage:           appsRollbackUsage,
		errContext:      "roll back",
		successFormat:   "app %q rolling back to image %q; reconcile is asynchronous, check \"%s apps status %s\"\n",
		knownImagesOnly: true,
	})
}

// checkRollbackTarget rejects image when it shares the app's image repo but
// is not one of its locally present tags. A different repo, or an empty or
// unreadable tag list (a remote node, a registry image), is let through.
func checkRollbackTarget(ctx context.Context, client *Client, prog, name, image string) error {
	images, err := client.ListAppImages(ctx, name)
	if err != nil || len(images) == 0 {
		return nil
	}
	want := unpinnedImage(image)
	if imageRepoOf(want) != imageRepoOf(unpinnedImage(images[0].Tag)) {
		return nil
	}
	known := make([]string, 0, len(images))
	for _, img := range images {
		if unpinnedImage(img.Tag) == want {
			return nil
		}
		known = append(known, img.Tag)
	}
	return newValidationError("image %q is not a locally built tag of app %q, and rolling back to it would stop the app; known tags: %s. Roll back by deploy id with \"%s apps deploys rollback-to %s <deploy-id>\", or use \"%s apps deploy\" for a registry image",
		image, name, strings.Join(known, ", "), prog, name, prog)
}

// imageRepoOf strips the tag from an unpinned image reference, leaving a
// registry port such as host:5000 intact.
func imageRepoOf(ref string) string {
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[:i]
	}
	return ref
}

func appsRollbackUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps rollback <name> --image IMAGE [flags]

Points an existing app's desired image back at an older, already-built
IMAGE and returns immediately; the next reconcile converges the running
container to it. This sends the same request "%[1]s apps deploy <name>
--image IMAGE" does. "%[1]s apps images <name>" lists the tags you can
roll back to, and "%[1]s apps deploys rollback-to <name> <deploy-id>"
rolls back to a past deploy pinned by digest.

A tag under the app's own image repo that is not locally present is
refused, since a container that cannot be created takes the app offline.
Exit 0 only means the server accepted the request; check "%[1]s apps
status <name>" to confirm the rollback landed.

If the app is tagged with a protected environment, this fails unless
--confirm is set or you type "yes" at the interactive prompt.

Flags:
  --image string          older, already-built image reference to roll back to (required)
  --confirm                  confirm rolling back into a protected environment, skipping the interactive prompt
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
