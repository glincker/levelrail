package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsPromote implements "apps promote <name> --to <environment-id>
// [--target <app-name>] [--preview]": moves name's current image onto a
// sibling app tagged with --to within the same project
// (internal/api/promote.go's own doc comment on why that, not "the same
// logical app in another environment", is the real constraint), through
// the same deploy path "apps deploy"/"apps rollback" already use.
// --target disambiguates when more than one app in --to belongs to the
// project; omit it to let the server auto-discover the sole candidate.
//
// --confirm/interactive-prompt behavior when --to is protected mirrors
// apps_deploy.go's own doc comment: same server-side gate
// (environmentNeedsConfirmation, internal/api/promote.go), same
// client-side interactive fallback (resolveProtectedEnvironmentConfirmation).
func runAppsPromote(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps promote", "print the result as JSON to stdout and nothing else", stderr)
	var to, target string
	var preview, confirm bool
	fs.StringVar(&to, "to", "", "destination environment ID (required)")
	fs.StringVar(&target, "target", "", "target app name, when more than one app in --to belongs to the same project")
	fs.BoolVar(&preview, "preview", false, "show what promoting would change, without applying it")
	fs.BoolVar(&preview, "dry-run", false, "alias for --preview")
	fs.BoolVar(&confirm, "confirm", false, "confirm promoting into a protected environment; omit to be prompted interactively if needed")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPromoteUsage(prog)) }

	client, name, jsonOut, exitCode, ok := parseSingleArgClient(fs, args, tokenFlagP, apiURLFlagP, jsonOutP, stderr, prog, "apps promote", lookupEnv)
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
		if jsonOut {
			if err := writeJSONValue(stdout, prev); err != nil {
				_, _ = fmt.Fprintln(stderr, err)
				return exitNetwork
			}
			return exitOK
		}
		printPromotePreviewHuman(stdout, prev)
		return exitOK
	}

	updated, err := client.PromoteApp(ctx, name, promoteAppRequest{To: to, Target: target, Confirm: confirm})
	if !confirm {
		if apiErr, ok := protectedEnvironmentError(err); ok {
			confirmed, cerr := resolveProtectedEnvironmentConfirmation(apiErr.Message, stdin, stderr)
			if cerr != nil {
				return reportError(stdout, stderr, jsonOut, cerr)
			}
			if confirmed {
				updated, err = client.PromoteApp(ctx, name, promoteAppRequest{To: to, Target: target, Confirm: true})
			}
		}
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("promote app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "app %q promoted onto %q (now image %q); reconcile is asynchronous, check \"%s apps status %s\"\n", name, updated.Name, updated.Image, prog, updated.Name)
	printAppHuman(stdout, updated)
	return exitOK
}

// printPromotePreviewHuman renders a promotion preview: which two apps
// are involved, what would change, and the same explicit not-tracked
// note printDeployCompareHuman (apps_deploys.go) prints, since this diff
// carries the identical honest limitation, only image is ever compared.
func printPromotePreviewHuman(w io.Writer, prev promotePreviewResource) {
	_, _ = fmt.Fprintf(w, "promote %q -> %q (environment %q)\n", prev.SourceApp, prev.TargetApp, prev.Environment.Name)
	_, _ = fmt.Fprintf(w, "  from: %s (image %s)\n", prev.From.AppName, prev.From.Image)
	_, _ = fmt.Fprintf(w, "  to:   %s (image %s)\n", prev.To.AppName, prev.To.Image)

	if len(prev.Changes) == 0 {
		_, _ = fmt.Fprint(w, "\nno change: target already runs this image\n")
	} else {
		_, _ = fmt.Fprint(w, "\nchanges:\n")
		for _, c := range prev.Changes {
			_, _ = fmt.Fprintf(w, "  %-10s %s -> %s\n", c.Field, deployCompareValueLabel(c.From), deployCompareValueLabel(c.To))
		}
	}

	_, _ = fmt.Fprintf(w, "\nnot compared: %v\n%s\n", prev.UnsnapshottedFields, prev.Note)
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
  --preview                 show what would change, don't apply it
  --dry-run                 alias for --preview
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the result as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
