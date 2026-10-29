package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// --target is only needed when more than one app in --to belongs to the project.
// Protected-environment confirmation matches apps deploy.
func runAppsPromote(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps promote", "print the result as JSON to stdout and nothing else", stderr)
	var to, target string
	var preview, confirm, includeEnv, force, overrideFreeze bool
	var overrideReason, idemKey, releaseID string
	fs.BoolVar(&includeEnv, "include-env", false, "also apply added and removed plain env keys (values of keys both apps have stay as they are)")
	fs.BoolVar(&force, "force", false, "promote even if the source app is unhealthy or its last deploy failed")
	fs.BoolVar(&overrideFreeze, "override-freeze", false, "promote even though a deploy freeze window is active")
	fs.StringVar(&overrideReason, "override-reason", "", "required with --override-freeze, recorded on the deploy")
	fs.StringVar(&idemKey, "idempotency-key", "", "override the derived Idempotency-Key so a retried run is applied once")
	fs.StringVar(&releaseID, "release", "", "promote this release id of the app, from \"releases list\" (server needs APP_RELEASE_BINDINGS=on)")
	fs.StringVar(&to, "to", "", "destination environment ID (required)")
	fs.StringVar(&target, "target", "", "target app name, when more than one app in --to belongs to the same project")
	fs.BoolVar(&preview, "preview", false, "show what promoting would change, without applying it")
	fs.BoolVar(&preview, "dry-run", false, "alias for --preview")
	fs.BoolVar(&confirm, "confirm", false, "confirm promoting into a protected environment; omit to be prompted interactively if needed")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPromoteUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps promote", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if to == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--to is required"))
	}

	ctx := context.Background()

	if preview {
		prev, err := client.PreviewPromotion(ctx, name, to, target)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview promotion for app %q: %w", name, err))
		}
		if err := renderResult(stdout, of.Format, of.Query, prev, func() { printPromotePreviewHuman(stdout, prev) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}

	if idemKey == "" {
		idemKey = idempotencyKey(lookupEnv, time.Now(), "promote", name, to, target)
	}
	ctx = apiclient.WithIdempotencyKey(ctx, idemKey)
	result, err := confirmProtectedEnvironment(confirm, stdin, stderr, func(confirm bool) (deployTriggerResult, error) {
		return client.PromoteApp(ctx, name, promoteAppRequest{To: to, Target: target, Confirm: confirm, IncludeEnv: includeEnv, Force: force, ReleaseID: releaseID, OverrideFreeze: overrideFreeze, OverrideReason: overrideReason})
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("promote app %q: %w", name, err))
	}

	if result.PendingApproval != nil {
		approval := *result.PendingApproval
		if err := renderResult(stdout, of.Format, of.Query, approval, func() {
			printDeployApprovalPendingHuman(stderr, prog, approval)
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}

	updated := *result.AppResource
	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stderr, "app %q promoted onto %q (now image %q); reconcile is asynchronous, check \"%s apps status %s\"\n", name, updated.Name, updated.Image, prog, updated.Name)
		printAppHuman(stdout, updated)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// Only image is compared, so the preview states that limitation explicitly.
func printPromotePreviewHuman(w io.Writer, prev promotePreviewResource) {
	_, _ = fmt.Fprintf(w, "promote %q -> %q (environment %q)\n", prev.SourceApp, prev.TargetApp, prev.Environment.Name)
	_, _ = fmt.Fprintf(w, "  from: %s (image %s)\n", prev.From.AppName, prev.From.Image)
	_, _ = fmt.Fprintf(w, "  to:   %s (image %s)\n", prev.To.AppName, prev.To.Image)
	if prev.From.ImageDigest != "" || prev.To.ImageDigest != "" {
		_, _ = fmt.Fprintf(w, "  digest: %s -> %s\n", deployCompareValueLabel(prev.To.ImageDigest), deployCompareValueLabel(prev.From.ImageDigest))
	}

	if len(prev.Changes) == 0 {
		_, _ = fmt.Fprint(w, "\nno change: target already runs this image\n")
	} else {
		_, _ = fmt.Fprint(w, "\nchanges:\n")
		for _, c := range prev.Changes {
			_, _ = fmt.Fprintf(w, "  %-10s %s -> %s\n", c.Field, deployCompareValueLabel(c.From), deployCompareValueLabel(c.To))
		}
	}

	d := prev.Diff
	for _, f := range []*apiclient.DeployCompareField{d.Replicas, d.Resources, d.Health} {
		if f != nil {
			_, _ = fmt.Fprintf(w, "  %-10s %s -> %s\n", f.Field, f.From, f.To)
		}
	}
	printKeyList(w, "env keys to add", d.EnvAdded)
	printKeyList(w, "env keys to remove", d.EnvRemoved)
	printKeyList(w, "env keys with different values (kept)", d.EnvChanged)
	printKeyList(w, "secret keys only on source", d.SecretsAdded)
	printKeyList(w, "secret keys only on target", d.SecretsGone)
	_, _ = fmt.Fprintf(w, "\nleft untouched: %s\n", strings.Join(d.Untouched, ", "))
	if prev.NeedsConfirmation {
		_, _ = fmt.Fprint(w, "\nthis environment needs --confirm\n")
	}
	if prev.Frozen {
		_, _ = fmt.Fprintf(w, "frozen: %s (use --override-freeze with --override-reason to promote anyway)\n", prev.FreezeReason)
	}
	for _, b := range prev.Blockers {
		_, _ = fmt.Fprintf(w, "blocked: %s (use --force to override)\n", b)
	}
}

func printKeyList(w io.Writer, label string, keys []string) {
	if len(keys) > 0 {
		_, _ = fmt.Fprintf(w, "  %s: %s\n", label, strings.Join(keys, ", "))
	}
}

func appsPromoteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--preview] [flags]

Points a sibling app's image at <name>'s current image and redeploys it,
through the same mechanism "%[1]s apps deploy" uses. The sibling app is
found in the project <name> belongs to: --target names it explicitly, or
it's auto-discovered when exactly one app in ENVIRONMENT_ID belongs to
that project. --preview (or --dry-run) shows the change without applying
it, the same "only the image tag is compared" honesty
"%[1]s apps deploys compare" already gives: environment variables,
ports, domains, and other service configuration are never part of this
diff.

If ENVIRONMENT_ID is protected, this fails unless --confirm is set or
you type "yes" at the interactive prompt.

Flags:
  --to string             destination environment ID (required)
  --target string        target app name, to disambiguate multiple apps in --to
  --confirm                  confirm promoting into a protected environment, skipping the interactive prompt
  --override-freeze          promote even though a deploy freeze window is active
  --override-reason string   required with --override-freeze, recorded on the deploy
  --include-env              also apply added and removed plain env keys (values never overwrite the target)
  --release string           promote this release id of the app instead of its bound one
  --force                    promote even if the source is unhealthy or its last deploy failed
  --preview                 show what would change, don't apply it
  --dry-run                 alias for --preview
  --idempotency-key string   override the derived Idempotency-Key (default: from app, image and CI run id)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
