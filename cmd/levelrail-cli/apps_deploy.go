package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type deployOrRollbackConfig struct {
	cmdLabel      string
	imageHelp     string
	confirmHelp   string
	usage         func(string) string
	errContext    string
	successFormat string
	// knownImagesOnly refuses a tag the app never built locally, since a
	// typo'd rollback target fails to create and takes the app offline.
	knownImagesOnly bool
	// fromSource registers --commit/--latest, deploying the connected git source.
	fromSource bool
}

// The reconcile converges the container asynchronously, so this returns once the 202 is saved.
// Rollback is the same call with an older tag, since the server has no separate rollback endpoint.
func runAppsDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	return runAppsDeployOrRollback(prog, args, stdout, stderr, lookupEnv, stdin, deployOrRollbackConfig{
		cmdLabel:      "apps deploy",
		imageHelp:     "image reference to deploy, e.g. registry.example.com/org/app:tag (required)",
		confirmHelp:   "confirm deploying into a protected environment; omit to be prompted interactively if needed",
		usage:         appsDeployUsage,
		errContext:    "deploy",
		fromSource:    true,
		successFormat: "app %q now targets image %q; reconcile is asynchronous, check \"%s apps status %s\"\n",
	})
}

// runAppsDeployOrRollback is runAppsDeploy/runAppsRollback's shared
// implementation: see runAppsDeploy's own doc comment for why there are
// two commands over one underlying mechanism.
func runAppsDeployOrRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader, cfg deployOrRollbackConfig) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cfg.cmdLabel, "print the updated app as JSON to stdout and nothing else", stderr)
	var image, idemKey, at, releaseID string
	var confirm, diff bool
	var extra deployExtraFlags
	fs.StringVar(&image, "image", "", cfg.imageHelp)
	fs.BoolVar(&confirm, "confirm", false, cfg.confirmHelp)
	fs.BoolVar(&diff, "diff", false, "show what this deploy would change (config, env keys, image) against the last successful deploy and exit without deploying")
	fs.StringVar(&idemKey, "idempotency-key", "", "override the derived Idempotency-Key so a retried run is applied once")
	extra.register(fs)
	if cfg.knownImagesOnly {
		fs.StringVar(&releaseID, "release", "", "roll back to this release id of the app, from \"releases list\" (needs release bindings enabled on the server)")
	}
	var src sourceDeployFlags
	if cfg.fromSource {
		fs.StringVar(&at, "at", "", "schedule the deploy once instead of running it now: an RFC 3339 time or \"in 2h\"; use with --image, or --latest --branch NAME")
		fs.StringVar(&src.commit, "commit", "", "deploy this commit of the connected git source (builds it, no --image needed)")
		fs.BoolVar(&src.latest, "latest", false, "deploy the newest commit of the connected branch")
		fs.StringVar(&src.branch, "branch", "", "with --latest: branch to take the newest commit from (default: the connected branch)")
		fs.BoolVar(&src.fromCache, "from-cache", false, "rebuild from the control plane's source cache without contacting the git host (default commit: the last successful source deploy)")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cfg.usage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
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

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if at != "" {
		if diff || src.fromCache || src.commit != "" || (image != "") == src.latest {
			return reportError(stdout, stderr, jsonOut, newValidationError("--at needs exactly one of --image or --latest --branch NAME"))
		}
		if src.latest && src.branch == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--at with --latest needs --branch NAME"))
		}
		if idemKey == "" {
			idemKey = idempotencyKey(lookupEnv, time.Now(), "schedule", name, image+src.branch+at)
		}
		return scheduleDeployAt(prog, client, name, at, image, src.branch, idemKey, stdout, stderr, jsonOut, of)
	}

	if diff {
		return showPendingDeployDiff(ctx, client, name, image, stdout, stderr, jsonOut, of)
	}

	if src.fromCache {
		if image != "" || src.latest || src.branch != "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--from-cache takes only an optional --commit"))
		}
		return deployFromCache(prog, client, name, src.commit, extra, stdout, stderr, jsonOut, of)
	}
	if src.commit != "" || src.latest {
		if image != "" || (src.commit != "" && src.latest) {
			return reportError(stdout, stderr, jsonOut, newValidationError("use exactly one of --image, --commit, or --latest"))
		}
		return deployFromSource(prog, client, name, src, stdout, stderr, jsonOut, of)
	}
	if image == "" && extra.pull {
		current, err := client.GetApp(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("load app %q: %w", name, err))
		}
		image = unpinnedImage(current.Image)
	}
	if image == "" && cfg.knownImagesOnly && releaseID == "" {
		previous, err := previousDeployImage(ctx, client, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		_, _ = fmt.Fprintf(stderr, "rolling back to the previous successful deploy, image %s\n", previous)
		image = previous
	}
	if image == "" && releaseID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--image is required (or --pull to re-resolve the current tag)"))
	}
	if cfg.knownImagesOnly && releaseID == "" {
		resolved, err := checkRollbackTarget(ctx, client, prog, name, image)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		image = resolved
	}

	if idemKey == "" {
		idemKey = idempotencyKey(lookupEnv, time.Now(), cfg.errContext, name, image)
	}
	ctx = apiclient.WithIdempotencyKey(ctx, idemKey)
	result, err := confirmProtectedEnvironment(confirm, stdin, stderr, func(confirm bool) (deployTriggerResult, error) {
		return client.DeployAppWith(ctx, name, withRelease(extra.request(image, confirm), releaseID))
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s app %q: %w", cfg.errContext, name, err))
	}

	if result.PendingApproval != nil {
		approval := *result.PendingApproval
		return writeScheduledTaskResult(stdout, stderr, of, approval, func() {
			printDeployApprovalPendingHuman(stderr, prog, approval)
		})
	}

	updated := *result.AppResource
	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stderr, cfg.successFormat, updated.Name, updated.Image, prog, updated.Name)
		printAppHuman(stdout, updated)
	})
}

func withRelease(req apiclient.DeployTriggerRequest, releaseID string) apiclient.DeployTriggerRequest {
	req.ReleaseID = releaseID
	return req
}

func showPendingDeployDiff(ctx context.Context, client *Client, name, image string, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	cmp, err := client.PendingDeployDiff(ctx, name, image)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("diff deploy for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, cmp, func() { printDeployCompareHuman(stdout, cmp) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
  --commit string            build and deploy this commit of the app's connected git source
  --latest                   build and deploy the newest commit of the connected branch (with --branch to pick another)
  --from-cache               rebuild from the source cache without contacting the git host, for a git host outage
                             (default commit: the last successful source deploy; --commit picks another)
  --pull                     re-resolve the tag against its registry, failing rather than deploying a cached
                             image; without --image, redeploys the app's current tag at its newest digest
  --diff                     show what this deploy would change (config, env keys, image) against the last
                             successful deploy, then exit without deploying
  --at string                schedule the deploy once instead of running it now: an RFC 3339 time or "in 2h".
                             Use with --image, or with --latest --branch NAME (see "apps schedule list")
  --override-freeze          deploy even though a deploy freeze window is active
  --override-reason string   required with --override-freeze, recorded on the deploy
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
