package main

import (
	"context"
	"fmt"
	"io"
)

func runAppsPreviewsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps previews create", "print the preview as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsCreateUsage(prog)) }
	pr := fs.Int("pr", 0, "pull request number")
	image := fs.String("image", "", "image reference built by CI")
	branch := fs.String("branch", "", "head branch name, shown on the preview only")
	sha := fs.String("sha", "", "head commit SHA")

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps previews create", "app name")
	if !ok {
		return exitUsage
	}
	if *pr <= 0 || *image == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps previews create requires --pr and --image\n\n", prog)
		_, _ = fmt.Fprint(stderr, appsPreviewsCreateUsage(prog))
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	p, err := client.CreatePreviewFromImage(context.Background(), appName, *pr, *image, *branch, *sha)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create preview for app %q pr #%d: %w", appName, *pr, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() {
		_, _ = fmt.Fprintf(stdout, "preview %s %s %s\n", p.PreviewAppID, p.Status, p.Domain)
	})
}

func appsPreviewsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews create <app-name> --pr N --image REF [--branch B] [--sha S] [flags]

Deploys an image your CI already built as the preview for one pull request,
with no source fetch or build on the server. The app needs a connected git
source with previews enabled and a single service. Needs the deploy ability.

Flags:
  --pr int                pull request number
  --image string          image reference to deploy
  --branch string         head branch name, shown on the preview only (not used for env overrides or the host)
  --sha string            head commit SHA
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the preview as JSON to stdout and nothing else
  --output string         output format: json, ndjson, yaml, table, wide, text or name
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
