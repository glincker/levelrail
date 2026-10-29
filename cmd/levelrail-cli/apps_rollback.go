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
	if rest, found := takeBoolFlag(args, "--with-data"); found {
		return runAppsRollbackWithData(prog, rest, stdout, stderr, lookupEnv)
	}
	return runAppsDeployOrRollback(prog, args, stdout, stderr, lookupEnv, stdin, deployOrRollbackConfig{
		cmdLabel:        "apps rollback",
		imageHelp:       "older, already-built image reference to roll back to, e.g. registry.example.com/org/app:tag (default: the previous successful deploy)",
		confirmHelp:     "confirm rolling back into a protected environment; omit to be prompted interactively if needed",
		usage:           appsRollbackUsage,
		errContext:      "roll back",
		successFormat:   "app %q rolling back to image %q; reconcile is asynchronous, check \"%s apps status %s\"\n",
		knownImagesOnly: true,
	})
}

// previousDeployImage is the newest succeeded deploy whose image differs from the one the app runs now.
func previousDeployImage(ctx context.Context, client *Client, name string) (string, error) {
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return "", fmt.Errorf("get app %q: %w", name, err)
	}
	attempts, err := client.ListDeployAttempts(ctx, name)
	if err != nil {
		return "", fmt.Errorf("list deploys for app %q: %w", name, err)
	}
	current := unpinnedImage(app.Image)
	var newest *deployAttemptResource
	for i := range attempts {
		a := &attempts[i]
		if a.Status != "succeeded" || a.Image == "" || unpinnedImage(a.Image) == current {
			continue
		}
		if newest == nil || a.StartedAt.After(newest.StartedAt) {
			newest = a
		}
	}
	if newest == nil {
		return "", newValidationError("app %q has no earlier successful deploy to go back to; pass --image, see \"apps images %s\"", name, name)
	}
	return newest.Image, nil
}

// A history tag resolves to its recorded digest so the request names the exact image.
// Other repos, and unreadable tag lists (remote or registry images), pass through unchanged.
func checkRollbackTarget(ctx context.Context, client *Client, prog, name, image string) (string, error) {
	images, err := client.ListAppImages(ctx, name)
	if err != nil || len(images) == 0 {
		return image, nil
	}
	want := unpinnedImage(image)
	if imageRepoOf(want) != imageRepoOf(unpinnedImage(images[0].Tag)) {
		return image, nil
	}
	known := make([]string, 0, len(images))
	for _, img := range images {
		if unpinnedImage(img.Tag) == want {
			return image, nil
		}
		known = append(known, img.Tag)
	}
	attempts, _ := client.ListDeployAttempts(ctx, name)
	var newest *deployAttemptResource
	for i := range attempts {
		a := &attempts[i]
		if a.ID == "" || a.Image == "" {
			continue
		}
		if unpinnedImage(a.Image) == want {
			if a.ImageDigest != "" && !strings.Contains(image, "@") {
				return imageRepoOf(want) + "@" + a.ImageDigest, nil
			}
			return image, nil
		}
		if newest == nil || a.StartedAt.After(newest.StartedAt) {
			newest = a
		}
	}
	deployID := "<deploy-id>"
	if newest != nil {
		deployID = newest.ID
	}
	return "", newValidationError("image %q is not a locally built tag of app %q and is not in its deploy history, and rolling back to it would stop the app; known tags: %s. Roll back by deploy id with \"%s apps deploys rollback-to %s %s\", or use \"%s apps deploy\" for a registry image",
		image, name, strings.Join(known, ", "), prog, name, deployID, prog)
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
  %[1]s apps rollback <name> [--image IMAGE] [flags]

Without --image, rolls back to the app's previous successful deploy.
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
  --image string          older, already-built image reference to roll back to (default: the previous successful deploy)
  --confirm                  confirm rolling back into a protected environment, skipping the interactive prompt
  --release string           roll back to this release id of the app, from "releases list" (server needs APP_RELEASE_BINDINGS=on)
  --with-data                restore the pre-deploy database snapshot into new databases, swap the app's links and roll back the code (--deploy ID picks the deploy to undo)
  --diff                     show what this rollback would change against the last successful deploy, then exit without rolling back
  --idempotency-key string   override the derived Idempotency-Key (default: from app, image and CI run id)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
