package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsBuilds dispatches "apps builds <verb> [flags]" to trigger: POST
// /api/v1/apps/{name}/builds (internal/api/builds.go's own
// handleTriggerBuild), building an image from a git source and
// deploying it. Asynchronous, like a webhook-triggered deploy: the
// response is only a deploy_attempts id (beginBuildDeployAttempt), the
// fetch and build run in the background. There is no separate
// build-history listing: a triggered build's progress and outcome show
// up in the same deploy-attempts history "apps deploys list"/"apps
// deploys logs" already surface, so history stays on "apps deploys"
// rather than a second, redundant command here.
func runAppsBuilds(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBuildsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBuildsUsage(prog))
		return exitOK
	case "trigger":
		return runAppsBuildsTrigger(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps builds subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsBuildsUsage(prog))
		return exitUsage
	}
}

func appsBuildsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps builds trigger <name> --repo URL --ref REF [flags]

Builds an image from a git source and deploys it to an existing app, the
manual counterpart to a push-triggered auto-deploy ("apps git-source
set"). Asynchronous: returns a deploy attempt id immediately, the build
itself runs in the background. Poll it with "apps deploys list" or
follow its build log with "apps deploys logs <name> <id>".

Run "%[1]s apps builds <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsBuildsTrigger(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps builds trigger", "print the deploy attempt id as JSON to stdout and nothing else", stderr)
	var repo, ref, imageRepo, buildType, dockerfile, baseDirectory, image string
	buildArgs := map[string]string{}
	fs.StringVar(&repo, "repo", "", "git repository URL to build from (required)")
	fs.StringVar(&ref, "ref", "", "git ref (branch, tag, or commit) to build from (required)")
	fs.StringVar(&imageRepo, "image-repo", "", "image name without a tag, e.g. registry.example.com/org/app")
	fs.StringVar(&buildType, "build-type", "", "dockerfile (default), railpack, static, or image")
	fs.StringVar(&dockerfile, "dockerfile", "", "Dockerfile path relative to the repo root (--build-type dockerfile), or the built output directory (--build-type static)")
	fs.StringVar(&baseDirectory, "base-directory", "", "subdirectory of the repo to build from, for a monorepo")
	fs.StringVar(&image, "image", "", "prebuilt registry reference to deploy as-is (--build-type image only, no repo/ref build)")
	fs.Var(stringMapFlag(buildArgs), "build-arg", "Dockerfile build arg as KEY=VALUE, repeatable (--build-type dockerfile only)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps builds trigger <name> --repo URL --ref REF [flags]\n\nBuilds an image from a git source and deploys it to <name>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps builds trigger", "app name")
	if !ok {
		return exitUsage
	}
	if repo == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps builds trigger requires --repo\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if ref == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps builds trigger requires --ref\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	req := buildTriggerRequest{
		RepoURL:   repo,
		Ref:       ref,
		ImageRepo: imageRepo,
		Build: buildTriggerRequestBuild{
			Type:          buildType,
			Path:          dockerfile,
			BaseDirectory: baseDirectory,
			Image:         image,
		},
	}
	if len(buildArgs) > 0 {
		req.Build.Args = buildArgs
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.TriggerBuild(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("trigger build for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "build triggered for app %q, deploy attempt %q (check \"apps deploys list %s\" or \"apps deploys logs %s %s\")\n", name, result.ID, name, name, result.ID)
	})
}
