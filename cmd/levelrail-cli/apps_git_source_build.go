package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// setGitSourceBuild changes only the build settings of a connected source.
func setGitSourceBuild(ctx context.Context, client *apiclient.Client, name, buildType, dockerfile, baseDirectory string, static staticBuildFlags) (gitSourceResource, error) {
	if buildType == "" {
		current, err := client.GetGitSource(ctx, name)
		if err != nil {
			return gitSourceResource{}, fmt.Errorf("load git source for app %q: %w", name, err)
		}
		buildType = current.BuildType
	}
	command, publish, err := resolveStaticBuild(ctx, client, name, buildType, static)
	if err != nil {
		return gitSourceResource{}, err
	}
	return client.SetGitSourceBuild(ctx, name, apiclient.SetGitSourceBuildRequest{
		BuildType: buildType, BuildPath: dockerfile, BaseDirectory: baseDirectory, BuildCommand: command, PublishDir: publish, StaticSPA: static.SPA,
	})
}

// staticBuildFlags are the CLI's static build inputs; Infer asks the server
// to detect the framework and fill whichever of the two is empty.
type staticBuildFlags struct {
	Command string
	Publish string
	Infer   bool
	// SPA is nil to keep the stored value.
	SPA *bool
}

// resolveStaticBuild fills a missing command or publish directory from the
// repository's framework preset, so an override is optional.
func resolveStaticBuild(ctx context.Context, client *apiclient.Client, name, buildType string, f staticBuildFlags) (command, publish string, err error) {
	command, publish = f.Command, f.Publish
	if buildType != "static" || (command != "" && publish != "") || (command == "" && publish == "" && !f.Infer) {
		return command, publish, nil
	}
	det, err := client.DetectGitSourceBuild(ctx, name, "")
	if err != nil {
		return "", "", fmt.Errorf("detect the static build for app %q: %w", name, err)
	}
	if det.StaticBuild == nil {
		return "", "", fmt.Errorf("could not infer a static build for app %q: pass both --build-command and --publish-dir", name)
	}
	if command == "" {
		command = det.StaticBuild.Command
	}
	if publish == "" {
		publish = det.StaticBuild.PublishDir
	}
	return command, publish, nil
}

func printResolvedBuild(out io.Writer, gs gitSourceResource) {
	if gs.BaseDirectory != "" {
		_, _ = fmt.Fprintf(out, "base_directory: %s\n", gs.BaseDirectory)
	}
	if gs.ResolvedBuild.Summary != "" {
		_, _ = fmt.Fprintf(out, "builds:         %s\n", gs.ResolvedBuild.Summary)
	}
	if gs.BuildCommand != "" {
		_, _ = fmt.Fprintf(out, "build_command:  %s\npublish_dir:    %s\n", gs.BuildCommand, gs.PublishDir)
	}
	if gs.StaticSPA {
		_, _ = fmt.Fprintln(out, "spa:            on")
	}
}

func runAppsGitSourceDetect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source detect", "print the detection result as JSON to stdout and nothing else", stderr)
	var branch string
	var apply bool
	var pkgDir string
	fs.StringVar(&pkgDir, "package", "", "make this workspace package (its directory) the app's root and set its watch paths")
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
	if pkgDir != "" {
		gs, err := applyWorkspacePackage(ctx, client, name, det, strings.Trim(pkgDir, "/"))
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply package for app %q: %w", name, err))
		}
		applied = &gs
	} else if apply {
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

// applyWorkspacePackage points the app at one detected package: its directory
// as the build root and its watch paths as the deploy filter.
func applyWorkspacePackage(ctx context.Context, client *Client, name string, det apiclient.GitBuildDetection, dir string) (gitSourceResource, error) {
	for _, p := range det.Packages {
		if p.Dir != dir {
			continue
		}
		req := apiclient.SetGitSourceBuildRequest{BuildType: "railpack", BaseDirectory: p.Dir}
		if p.Dockerfile != "" {
			req.BuildType, req.BuildPath = "dockerfile", p.Dockerfile
		}
		gs, err := client.SetGitSourceBuild(ctx, name, req)
		if err != nil {
			return gs, err
		}
		paths := p.WatchPaths
		if _, err := client.SetGitDeploySettings(ctx, name, apiclient.SetGitDeploySettingsRequest{DeployPaths: &paths}); err != nil {
			return gs, fmt.Errorf("set watch paths: %w", err)
		}
		return gs, nil
	}
	return gitSourceResource{}, fmt.Errorf("no workspace package at %q was detected", dir)
}

func runAppsGitSourceAffected(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source affected", "print the verdict as JSON to stdout and nothing else", stderr)
	var branch, files string
	fs.StringVar(&branch, "branch", "", "branch to inspect (default: the source's branch)")
	fs.StringVar(&files, "files", "", "comma-separated changed paths; include both sides of a rename")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source affected <name> --files a,b [flags]\n\nDry run: reports which workspace packages, and whether this app, a change\nto those paths would redeploy.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source affected", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	var list []string
	for _, f := range strings.Split(files, ",") {
		if f = strings.TrimSpace(f); f != "" {
			list = append(list, f)
		}
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: --files is required\n", prog)
		return exitUsage
	}
	res, err := client.GitSourceAffected(context.Background(), name, branch, list)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check affected paths for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "this app deploys: %t\n", res.AppDeploys)
		if res.Reason != "" {
			_, _ = fmt.Fprintf(stdout, "reason:           %s\n", res.Reason)
		}
		for _, p := range res.Packages {
			_, _ = fmt.Fprintf(stdout, "  %-30s affected=%t\n", p.Dir, p.Affected)
		}
	}); err != nil {
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
	if len(det.Packages) > 0 {
		_, _ = fmt.Fprintln(out, "packages:")
		for _, p := range det.Packages {
			_, _ = fmt.Fprintf(out, "   %s (%s) watch: %s\n", p.Dir, p.Tool, strings.Join(p.WatchPaths, ", "))
		}
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
