package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// setGitSourceBuild changes only the build settings of a connected source.
func setGitSourceBuild(ctx context.Context, client *apiclient.Client, name, buildType, dockerfile, baseDirectory string) (gitSourceResource, error) {
	if buildType == "" {
		current, err := client.GetGitSource(ctx, name)
		if err != nil {
			return gitSourceResource{}, fmt.Errorf("load git source for app %q: %w", name, err)
		}
		buildType = current.BuildType
	}
	return client.SetGitSourceBuild(ctx, name, apiclient.SetGitSourceBuildRequest{
		BuildType: buildType, BuildPath: dockerfile, BaseDirectory: baseDirectory,
	})
}

func printResolvedBuild(out io.Writer, gs gitSourceResource) {
	if gs.BaseDirectory != "" {
		_, _ = fmt.Fprintf(out, "base_directory: %s\n", gs.BaseDirectory)
	}
	if gs.ResolvedBuild.Summary != "" {
		_, _ = fmt.Fprintf(out, "builds:         %s\n", gs.ResolvedBuild.Summary)
	}
}

func runAppsGitSourceDetect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source detect", "print the detection result as JSON to stdout and nothing else", stderr)
	var branch string
	var apply bool
	fs.StringVar(&branch, "branch", "", "branch to inspect (default: the source's branch)")
	fs.BoolVar(&apply, "apply", false, "apply the recommended suggestion as the app's build settings")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source detect <name> [flags]\n\nLists candidate build roots and Dockerfiles in the connected repository,\nwithout cloning it for GitHub, GitLab and Gitea. Paths are relative to the\nrepository root.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source detect", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	ctx := context.Background()
	det, err := client.DetectGitSourceBuild(ctx, name, branch)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("detect build settings for app %q: %w", name, err))
	}
	var applied *gitSourceResource
	if apply {
		if len(det.Suggestions) == 0 {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("detect build settings for app %q: nothing to apply, no suggestions were found", name))
		}
		top := det.Suggestions[0]
		gs, err := client.SetGitSourceBuild(ctx, name, apiclient.SetGitSourceBuildRequest{
			BuildType: top.BuildType, BuildPath: top.DockerfilePath, BaseDirectory: top.BaseDirectory,
		})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply build settings for app %q: %w", name, err))
		}
		applied = &gs
	}

	if err := renderResult(stdout, of.Format, of.Query, det, func() { printDetectionHuman(stdout, det, applied) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printDetectionHuman(out io.Writer, det apiclient.GitBuildDetection, applied *gitSourceResource) {
	_, _ = fmt.Fprintf(out, "branch:         %s\n", det.Branch)
	_, _ = fmt.Fprintf(out, "monorepo:       %t\n", det.LooksLikeMonorepo)
	if len(det.Tools) > 0 {
		_, _ = fmt.Fprintf(out, "tooling:        %s\n", strings.Join(det.Tools, ", "))
	}
	if det.NeedsBuildSettings {
		_, _ = fmt.Fprintln(out, "attention:      this looks like a monorepo and there is no app at the repository root, pick a Dockerfile below")
	}
	if len(det.Suggestions) == 0 {
		_, _ = fmt.Fprintln(out, "suggestions:    none found")
		return
	}
	_, _ = fmt.Fprintln(out, "suggestions:")
	for i, s := range det.Suggestions {
		mark := " "
		if s.Recommended {
			mark = "*"
		}
		_, _ = fmt.Fprintf(out, " %s %d. type=%s", mark, i+1, s.BuildType)
		if s.DockerfilePath != "" {
			_, _ = fmt.Fprintf(out, " dockerfile=%s", s.DockerfilePath)
		}
		ctxDir := s.BaseDirectory
		if ctxDir == "" {
			ctxDir = "."
		}
		_, _ = fmt.Fprintf(out, " context=%s\n      %s\n", ctxDir, s.Reason)
	}
	if applied != nil {
		_, _ = fmt.Fprintf(out, "applied:        %s\n", applied.ResolvedBuild.Summary)
	}
}
