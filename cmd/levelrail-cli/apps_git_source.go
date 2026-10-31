package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Multi-service git sources are dashboard-only; use 'apps deploy-spec' for them from the CLI.
func runAppsGitSource(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsGitSourceUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsGitSourceUsage(prog))
		return exitOK
	case "validate":
		return runAppsGitSourceValidate(prog, args[1:], stdout, stderr, lookupEnv)
	case "detect":
		return runAppsGitSourceDetect(prog, args[1:], stdout, stderr, lookupEnv)
	case "affected":
		return runAppsGitSourceAffected(prog, args[1:], stdout, stderr, lookupEnv)
	case "commits":
		return runAppsGitSourceCommits(prog, args[1:], stdout, stderr, lookupEnv)
	case "get", "show":
		return runAppsGitSourceGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsGitSourceSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "settings":
		return runAppsGitSourceSettings(prog, args[1:], stdout, stderr, lookupEnv)
	case "webhook":
		return runAppsGitSourceWebhook(prog, args[1:], stdout, stderr, lookupEnv)
	case "rotate-secret":
		return runAppsGitSourceRotateSecret(prog, args[1:], stdout, stderr, lookupEnv)
	case "register-webhook":
		return runAppsGitSourceRegisterWebhook(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsGitSourceDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps git-source subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsGitSourceUsage(prog))
		return exitUsage
	}
}

func appsGitSourceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps git-source get <name> [flags]                              show an app's connected repo
  %[1]s apps git-source show <name> [flags]                             same as get
  %[1]s apps git-source commits <name> [--branch B] [flags]             newest commits with deploy status and suggestions
  %[1]s apps git-source set <name> --repo-url URL [flags]              connect (or edit) a repo for auto-deploy-on-push
  %[1]s apps git-source set <name> --build-type T --dockerfile P --base-directory D   change only the build settings
  %[1]s apps git-source set <name> --build-type static [--build-command C --publish-dir D | --infer-build]   build a static site, then serve it
  %[1]s apps git-source validate <name> [flags]                         check repo access with the deploy credential and show the fix
  %[1]s apps git-source detect <name> [--apply|--package <dir>] [flags] list Dockerfiles, build roots and workspace packages
  %[1]s apps git-source affected <name> --files a,b [flags]             dry run: which packages a change would redeploy
  %[1]s apps git-source settings <name> [flags]                         set push path filters and forge status reporting
  %[1]s apps git-source webhook <name> [--rotate] [flags]               print the URL, content type and events for the provider
  %[1]s apps git-source rotate-secret <name> [flags]                    mint a fresh webhook secret, shown once
  %[1]s apps git-source register-webhook <name> [flags]                 register the push webhook on the provider again
  %[1]s apps git-source delete <name> [flags]                           disconnect an app's repo

Run "%[1]s apps git-source <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsGitSourceGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source get", "print the git source as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source get <name> [flags]\n\nShows an app's connected repo, if any.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source get", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	gs, err := client.GetGitSource(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get git source for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, gs, func() { printGitSourceHuman(stdout, gs) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsGitSourceSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source set", "print the git source as JSON to stdout and nothing else", stderr)
	var repoURL, branch, buildType, buildPath, token, triggerMode, baseDirectory string
	fs.StringVar(&repoURL, "repo-url", "", "repo URL to connect (required)")
	fs.StringVar(&branch, "branch", "", "branch to deploy on push (default: the server's default branch)")
	fs.StringVar(&buildType, "build-type", "", "dockerfile, railpack, or static (default: dockerfile)")
	fs.StringVar(&buildPath, "build-path", "", "Dockerfile path, relative to the repository root")
	fs.StringVar(&buildPath, "dockerfile", "", "alias for --build-path")
	fs.StringVar(&baseDirectory, "base-directory", "", "build context directory, relative to the repository root (default: the root)")
	var static staticBuildFlags
	fs.StringVar(&static.Command, "build-command", "", "static sites: command that builds the site in a build container, e.g. \"npm run build\"")
	fs.StringVar(&static.Publish, "publish-dir", "", "static sites: directory the build writes, relative to the base directory, e.g. dist")
	fs.BoolVar(&static.Infer, "infer-build", false, "static sites: detect the framework and fill the build command and publish directory")
	fs.BoolFunc("spa", "static sites: serve index.html for unknown paths, --spa=false turns it off", func(v string) error {
		on := v != "false" && v != "0"
		static.SPA = &on
		return nil
	})
	fs.StringVar(&token, "token-secret", "", "personal access token for a private repo; empty on an update leaves the stored token unchanged")
	fs.StringVar(&triggerMode, "trigger-mode", "", "push or release (default: push); release deploys only on a tag push or a published github release")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source set <name> --repo-url URL [flags]\n\nConnects a repo for auto-deploy-on-push, or edits an existing\nconnection. Multi-service fan-out (an app.yaml services: map) is\ndashboard-only; use \"apps deploy-spec\" from the CLI for that case.\n\nStatic sites can build before serving: --build-command and --publish-dir,\nor --infer-build to detect the framework and fill either one you omit.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source set", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if repoURL == "" && (buildType != "" || buildPath != "" || baseDirectory != "" || static != (staticBuildFlags{})) {
		gs, err := setGitSourceBuild(context.Background(), client, name, buildType, buildPath, baseDirectory, static)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set git source build for app %q: %w", name, err))
		}
		if err := renderResult(stdout, of.Format, of.Query, gs, func() { printGitSourceHuman(stdout, gs) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	if repoURL == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps git-source set requires --repo-url, or build flags to change only the build settings\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	if static.Infer {
		_, _ = fmt.Fprintf(stderr, "%s: --infer-build needs a connected repo, connect first and then run it with the build flags only\n", prog)
		return exitUsage
	}
	gs, err := client.SetGitSource(context.Background(), name, setGitSourceRequest{
		BuildCommand:  static.Command,
		PublishDir:    static.Publish,
		StaticSPA:     static.SPA != nil && *static.SPA,
		RepoURL:       repoURL,
		Branch:        branch,
		BuildType:     buildType,
		BuildPath:     buildPath,
		BaseDirectory: baseDirectory,
		Token:         token,
		TriggerMode:   triggerMode,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set git source for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, gs, func() { printGitSourceHuman(stdout, gs) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// Rotate changes only the webhook secret and leaves repo and build config intact. The new secret is shown once.
func runAppsGitSourceRotateSecret(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source rotate-secret", "print the git source (including the new webhook_secret) as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source rotate-secret <name> [flags]\n\nMints a fresh webhook secret without touching repo_url, branch, or build\nconfig. The old secret stops verifying immediately; paste the new one\ninto the git provider's webhook settings before the next delivery.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source rotate-secret", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	gs, err := client.RotateGitSourceWebhookSecret(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate git source webhook secret for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, gs, func() {
		printGitSourceHuman(stdout, gs)
		_, _ = fmt.Fprintf(stdout, "webhook_secret: %s (shown once, will not be shown again)\n", gs.WebhookSecret)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsGitSourceDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source delete <name> [flags]\n\nDisconnects an app's repo: pushes stop triggering an auto-deploy.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source delete", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.DeleteGitSource(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete git source for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "git source disconnected for app %q\n", name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printGitSourceHuman(out io.Writer, gs gitSourceResource) {
	_, _ = fmt.Fprintf(out, "app:            %s\n", gs.ServiceName)
	_, _ = fmt.Fprintf(out, "repo_url:       %s\n", gs.RepoURL)
	_, _ = fmt.Fprintf(out, "branch:         %s\n", gs.Branch)
	_, _ = fmt.Fprintf(out, "build_type:     %s\n", gs.BuildType)
	if gs.BuildPath != "" {
		_, _ = fmt.Fprintf(out, "build_path:     %s\n", gs.BuildPath)
	}
	printResolvedBuild(out, gs)
	_, _ = fmt.Fprintf(out, "trigger_mode:   %s\n", gs.TriggerMode)
	_, _ = fmt.Fprintf(out, "has_token:      %t\n", gs.HasToken)
	_, _ = fmt.Fprintf(out, "webhook_url:    %s\n", gs.WebhookURL)
	printGitSourceReliability(out, gs)
	_, _ = fmt.Fprintf(out, "preview_enabled: %t\n", gs.PreviewEnabled)
	_, _ = fmt.Fprintf(out, "post_pr_comments: %t\n", gs.PostPRComments)
	_, _ = fmt.Fprintf(out, "report_status:  %t\n", gs.ReportStatus)
	if len(gs.DeployPaths) > 0 {
		_, _ = fmt.Fprintf(out, "deploy_paths:   %s\n", strings.Join(gs.DeployPaths, ", "))
	}
	if len(gs.DeployPathsIgnore) > 0 {
		_, _ = fmt.Fprintf(out, "deploy_paths_ignore: %s\n", strings.Join(gs.DeployPathsIgnore, ", "))
	}
}

func runAppsGitSourceRegisterWebhook(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source register-webhook", "print the git source as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source register-webhook <name> [flags]\n\nRegisters the push webhook on the provider that hosts the repository, using\nthe connected provider app. Use it after a failed registration or after\nsetting the public URL. Exits non-zero when it still fails.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source register-webhook", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	gs, err := client.RegisterGitSourceWebhook(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("register git source webhook for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, gs, func() { printGitSourceHuman(stdout, gs) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if gs.WebhookRegistration == nil || gs.WebhookRegistration.State != "registered" {
		return exitCheckFailed
	}
	return exitOK
}

// printGitSourceReliability prints what a missed webhook would hinge on: an
// unreachable webhook address, a failed registration, and the poller's state.
func printGitSourceReliability(out io.Writer, gs gitSourceResource) {
	switch gs.WebhookURLWarning {
	case "private_address":
		_, _ = fmt.Fprintln(out, "warning:        the webhook address is private or local, git hosts cannot reach it; set a public URL (dashboard: Fix) and register the webhook again")
	case "no_public_url":
		_, _ = fmt.Fprintln(out, "warning:        no public address is configured, git hosts have no webhook address to call")
	}
	if r := gs.WebhookRegistration; r != nil {
		line := "webhook:        " + r.State
		if r.Error != "" {
			line += " (" + r.Error + ")"
		}
		_, _ = fmt.Fprintln(out, line)
	}
	if p := gs.Poll; p != nil {
		switch {
		case !p.Enabled:
			_, _ = fmt.Fprintln(out, "poll:           off")
		case p.LastPolledAt != nil:
			_, _ = fmt.Fprintf(out, "poll:           last checked %s\n", p.LastPolledAt.Format("2006-01-02 15:04:05Z07:00"))
		default:
			_, _ = fmt.Fprintln(out, "poll:           not checked yet")
		}
		if p.Error != "" {
			_, _ = fmt.Fprintf(out, "poll_error:     %s\n", p.Error)
		}
	}
}
