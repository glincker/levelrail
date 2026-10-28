package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/ghactions"
	"github.com/GLINCKER/levelrail/internal/githubapp"
)

const (
	githubReportTimeout    = 30 * time.Second
	defaultGitHubServerURL = "https://github.com"
)

// runAppsGitHubActions dispatches "apps github-actions get|set|start|finish <app>".
func runAppsGitHubActions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsGitHubActionsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsGitHubActionsUsage(prog))
		return exitOK
	case "get":
		return runAppsGitHubActionsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsGitHubActionsSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "workflow":
		return runAppsGitHubActionsWorkflow(prog, args[1:], stdout, stderr, lookupEnv)
	case "token":
		return runAppsGitHubActionsToken(prog, args[1:], stdout, stderr, lookupEnv)
	case "trust":
		return runAppsGitHubActionsTrust(prog, args[1:], stdout, stderr, lookupEnv)
	case "start":
		return runAppsGitHubActionsReport(prog, args[1:], stdout, stderr, lookupEnv, false)
	case "finish":
		return runAppsGitHubActionsReport(prog, args[1:], stdout, stderr, lookupEnv, true)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps github-actions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsGitHubActionsUsage(prog))
		return exitUsage
	}
}

func appsGitHubActionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps github-actions get <app> [flags]      show what the deploy action reports back to GitHub
  %[1]s apps github-actions set <app> [flags]      change the environment name and which reports are on
  %[1]s apps github-actions workflow <app> [flags] print a deploy workflow (OIDC login, wait, rollback on failure)
  %[1]s apps github-actions trust list|add|remove <app> [flags]   which repositories may deploy the app from CI (also "%[1]s apps ci trust")
  %[1]s apps github-actions token <app> [flags]    deprecated alias of "%[1]s apps ci token", which also takes --provider
  %[1]s apps github-actions start <app> [flags]    open a GitHub deployment and check run (used by the action)
  %[1]s apps github-actions finish <app> [flags]   close them with the deploy outcome (used by the action)

start and finish read GITHUB_TOKEN, GITHUB_REPOSITORY, GITHUB_SHA and
GITHUB_SERVER_URL from the environment. A failed GitHub call is a warning
on stderr, never a failed deploy.
`, prog)
}

func runAppsGitHubActionsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps github-actions get", "print the settings as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps github-actions get", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.GetAppGitHubActions(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get github actions settings for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() {
		_, _ = fmt.Fprintf(stdout, "%s: environment %s, deployments %v, checks %v\n", rest[0], s.Environment, s.ReportDeployments, s.ReportChecks)
	})
}

func runAppsGitHubActionsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps github-actions set", "print the saved settings as JSON to stdout and nothing else", stderr)
	var environment string
	var noDeployments, noChecks bool
	fs.StringVar(&environment, "environment", "", "GitHub environment name deployments are recorded under (default production)")
	fs.BoolVar(&noDeployments, "no-deployments", false, "do not record GitHub deployments")
	fs.BoolVar(&noChecks, "no-checks", false, "do not post a check run")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps github-actions set", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.SetAppGitHubActions(context.Background(), rest[0], apiclient.AppGitHubActionsResource{
		Environment: environment, ReportDeployments: !noDeployments, ReportChecks: !noChecks,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set github actions settings for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() {
		_, _ = fmt.Fprintf(stdout, "saved %s: environment %s, deployments %v, checks %v\n", rest[0], s.Environment, s.ReportDeployments, s.ReportChecks)
	})
}

type githubReportFlags struct {
	repo, sha, outcome, environment, environmentURL, logURL string
	deploymentID, checkRunID                                int64
	noDeployments, noChecks                                 bool
}

func runAppsGitHubActionsReport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), finish bool) int {
	sub := "apps github-actions start"
	if finish {
		sub = "apps github-actions finish"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, sub, "accepted for consistency; output is always key=value lines", stderr)
	var f githubReportFlags
	fs.StringVar(&f.repo, "repo", "", "owner/name (default $GITHUB_REPOSITORY)")
	fs.StringVar(&f.sha, "sha", "", "commit to report on (default $GITHUB_SHA)")
	fs.StringVar(&f.environment, "environment", "", "override the saved GitHub environment name")
	fs.StringVar(&f.environmentURL, "environment-url", "", "URL of the deployed app, shown on the GitHub deployment")
	fs.StringVar(&f.logURL, "log-url", "", "link to the workflow run (default built from $GITHUB_RUN_ID)")
	fs.BoolVar(&f.noDeployments, "no-deployments", false, "skip the GitHub deployment even if the app setting is on")
	fs.BoolVar(&f.noChecks, "no-checks", false, "skip the check run even if the app setting is on")
	if finish {
		fs.StringVar(&f.outcome, "outcome", "", "succeeded, failed, timeout or accepted (required)")
		fs.Int64Var(&f.deploymentID, "deployment-id", 0, "deployment id printed by start")
		fs.Int64Var(&f.checkRunID, "check-run-id", 0, "check run id printed by start")
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, sub, "an app name", 1)
	if !ok {
		return exitUsage
	}
	app := rest[0]

	env := func(k string) string { v, _ := lookupEnv(k); return v }
	repo, sha := firstNonEmptyString(f.repo, env("GITHUB_REPOSITORY")), firstNonEmptyString(f.sha, env("GITHUB_SHA"))
	owner, name, found := strings.Cut(repo, "/")
	ghToken := env("GITHUB_TOKEN")
	if !found || owner == "" || name == "" || sha == "" || ghToken == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("GITHUB_TOKEN, a owner/name repo and a commit sha are required (flags --repo and --sha, or the GitHub Actions environment)"))
	}
	if finish && f.outcome == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--outcome is required"))
	}

	settings := apiclient.AppGitHubActionsResource{Environment: "production", ReportDeployments: true, ReportChecks: true}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if s, err := client.GetAppGitHubActions(context.Background(), app); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: could not read GitHub report settings, using defaults: %v\n", err)
	} else {
		settings = s
	}
	if f.environment != "" {
		settings.Environment = f.environment
	}
	serverURL := firstNonEmptyString(env("GITHUB_SERVER_URL"), defaultGitHubServerURL)
	logURL := f.logURL
	if logURL == "" && env("GITHUB_RUN_ID") != "" {
		logURL = serverURL + "/" + repo + "/actions/runs/" + env("GITHUB_RUN_ID")
	}
	target := ghactions.Target{
		App: app, SHA: sha, Environment: settings.Environment, EnvironmentURL: f.environmentURL, LogURL: logURL,
		Production:  strings.EqualFold(settings.Environment, "production"),
		Deployments: settings.ReportDeployments && !f.noDeployments,
		Checks:      settings.ReportChecks && !f.noChecks,
	}
	reporter := &ghactions.Reporter{GH: githubapp.NewClient(), InstanceURL: serverURL, Token: ghToken, Owner: owner, Repo: name}
	ctx, cancel := context.WithTimeout(context.Background(), githubReportTimeout)
	defer cancel()

	if !finish {
		res := reporter.Start(ctx, target)
		printGitHubWarnings(stderr, res.Warnings)
		_, _ = fmt.Fprintf(stdout, "deployment_id=%s\ncheck_run_id=%s\n", idString(res.IDs.DeploymentID), idString(res.IDs.CheckRunID))
		return exitOK
	}
	res, err := reporter.Finish(ctx, target, ghactions.IDs{DeploymentID: f.deploymentID, CheckRunID: f.checkRunID}, f.outcome)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
	}
	printGitHubWarnings(stderr, res.Warnings)
	return exitOK
}

func printGitHubWarnings(stderr io.Writer, warnings []error) {
	for _, w := range warnings {
		_, _ = fmt.Fprintf(stderr, "warning: GitHub report failed (the deploy is unaffected): %v\n", w)
	}
}

func idString(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
