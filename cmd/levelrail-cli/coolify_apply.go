package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// Secrets cannot go in the create body (appResource has no field for them), so they are set afterward.
// validateAppResource needs a non-empty image at create time, so pendingImageTag stands in before any build.
func applyMigration(ctx context.Context, client *Client, report *migrationReport) {
	applyMigrationJournaled(ctx, client, report, nil)
}

// applyMigrationJournaled is applyMigration with each step recorded in jr, so a resume skips what is done.
func applyMigrationJournaled(ctx context.Context, client *Client, report *migrationReport, jr *applyJournal) {
	for i := range report.Apps {
		a := &report.Apps[i]
		if a.Blocking {
			continue
		}
		if a.Compose != nil {
			// Staged by the pipeline before apply, since staging writes files only.
			continue
		}
		if jr.failed() {
			a.CreateError = "not attempted: the apply journal could not be written"
			continue
		}
		rec, proceed := jr.beginApp(ctx, client, a)
		if !proceed {
			continue
		}
		name := a.ServiceName
		if rec.needsRun(stepCreated) {
			created, err := client.CreateApp(ctx, migrationAppBody(a))
			if err != nil {
				a.CreateError = fmt.Errorf("create app %q: %w", a.ServiceName, err).Error()
				jr.record(rec, stepCreated, stateFailed)
				continue
			}
			name = created.Name
			jr.markCreated(ctx, client, rec)
		}
		a.Applied = true
		if rec.state(stepGitSource) == stateDone {
			a.gitSourceSet = true
		}
		jr.runStep(ctx, client, rec, stepGitSource, func() string { return applyGitBuild(ctx, client, name, a) })
		jr.runStep(ctx, client, rec, stepNode, func() string { return applyPlacement(ctx, client, name, a) })
		jr.runStep(ctx, client, rec, stepIngress, func() string { return ingressOutcome(ctx, client, name, a) })
		jr.runStep(ctx, client, rec, stepPreviews, func() string { return applyPreviews(ctx, client, name, a) })
		jr.runStep(ctx, client, rec, stepSecrets, func() string {
			for key, value := range a.Service.EnvLiteral {
				buildTime := slices.Contains(a.Service.BuildTimeKeys, key)
				if err := client.SetSecretWithBuildTime(ctx, name, key, value, false, buildTime); err != nil {
					a.SecretsFailed = append(a.SecretsFailed, key)
					continue
				}
				a.SecretsSet = append(a.SecretsSet, key)
			}
			sort.Strings(a.SecretsSet)
			sort.Strings(a.SecretsFailed)
			return secretsOutcome(a)
		})
		jr.runStep(ctx, client, rec, stepFiles, func() string { return applyFiles(ctx, client, name, a) })
		jr.seal(ctx, client, rec)
	}
}

func migrationAppBody(a *mappedApp) appResource {
	body := appResource{
		Name:    a.ServiceName,
		Image:   pendingImageTag(a.ServiceName),
		Port:    a.Service.Port,
		Domains: a.Service.Domains,
	}
	if a.Service.Memory != "" || a.Service.CPU > 0 {
		if res, err := toServiceResources(&spec.Resources{Memory: a.Service.Memory, CPU: a.Service.CPU}); err == nil {
			body.Resources = res
		}
	}
	if a.Service.Health != nil {
		if h, err := toServiceHealth(healthMappedToSpec(*a.Service.Health)); err == nil {
			body.Health = h
		}
	}
	if h := a.Service.Hooks; h != nil {
		body.Hooks = &apiclient.ServiceHooks{PreDeploy: h.PreDeploy, PostDeploy: h.PostDeploy}
	}
	return body
}

func applyPreviews(ctx context.Context, client *Client, name string, a *mappedApp) string {
	pv := a.Service.Previews
	if pv == nil {
		return stateSkipped
	}
	before := len(a.Issues)
	req := apiclient.SetPreviewPolicyRequest{}
	if pv.MaxPreviews > 0 {
		req.MaxPreviews = &pv.MaxPreviews
	}
	if !gatePreviews(a, &req) {
		a.Issues = appendIssue(a.Issues, "previews", issueReview, "previews were not turned on: the source used basic auth and a gate password could not be generated")
	} else if _, err := client.SetPreviewPolicy(ctx, name, req); err != nil {
		a.Issues = append(a.Issues, migrationIssue{Field: "previews", Severity: issueReview, Detail: "preview policy was not applied: " + err.Error()})
	}
	return issueOutcome(false, before, a)
}

// applyFiles sets each copied file; a secret one goes through the encrypted store.
func applyFiles(ctx context.Context, client *Client, name string, a *mappedApp) string {
	if len(a.Files) == 0 {
		return stateSkipped
	}
	state := stateDone
	for _, f := range a.Files {
		req := apiclient.SetAppFileRequest{Path: f.Path, Content: string(f.Content), Secret: f.Secret}
		if _, err := client.SetAppFile(ctx, name, req, false); err != nil {
			a.Issues = append(a.Issues, migrationIssue{Field: "file_storages", Severity: issueReview, Detail: "file " + f.Path + " was not set: " + err.Error()})
			state = stateFailed
		}
	}
	return state
}

// applyGitBuild records the repo URL and mapped build settings without a token. Connecting does not
// clone, so a private repo only fails on the first deploy; a URL with credentials is never sent.
func applyGitBuild(ctx context.Context, client *Client, name string, a *mappedApp) string {
	svc := a.Service
	if svc.StartCommand != "" {
		a.Issues = append(a.Issues, migrationIssue{Field: "start_command", Severity: issuePlanned, Detail: "the start command is in the generated app.yaml (command:), it is not set through the API on create"})
	}
	if a.RepoURL == "" {
		return stateSkipped
	}
	planned := func(detail string) {
		a.Issues = append(a.Issues, migrationIssue{Field: "git_repository", Severity: issuePlanned, Detail: detail})
	}
	u, err := url.Parse(a.RepoURL)
	switch {
	case err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ssh"):
		planned("the repository is not an http, https or ssh URL, connect it on the app's git source page with the build settings from this report")
		return stateSkipped
	case u.User != nil:
		a.RepoURL = ""
		planned("the repository URL embeds credentials, so it was not sent or kept; connect it on the app's git source page with a deploy token instead")
		return stateSkipped
	}
	planned("verify repository access: connecting does not clone, so give a private repository a token on the git source page before the first deploy")
	req := apiclient.SetGitSourceRequest{RepoURL: a.RepoURL, Branch: a.Ref, BuildType: svc.BuildType, BuildPath: svc.BuildPath,
		BaseDirectory: svc.BaseDirectory, BuildCommand: svc.BuildCommand, PublishDir: svc.PublishDir}
	if _, err := client.SetGitSource(ctx, name, req); err != nil {
		a.Issues = append(a.Issues, migrationIssue{Field: "git_repository", Severity: issuePlanned, Detail: "git source and build settings were not applied (" + err.Error() + "), connect the repository by hand with the build settings from this report"})
		return stateFailed
	}
	a.gitSourceSet = true
	return stateDone
}

func healthMappedToSpec(h mappedHealth) *spec.Health {
	probe := spec.Probe{Path: h.Path, Failures: h.Failures}
	if h.Interval > 0 {
		probe.Interval = fmt.Sprintf("%ds", h.Interval)
	}
	if h.Timeout > 0 {
		probe.Timeout = fmt.Sprintf("%ds", h.Timeout)
	}
	return &spec.Health{Readiness: &probe, Liveness: &probe}
}

// defaultStageDir is where staged compose apps go. Staging writes files only,
// because the compose deploy route starts containers.
const defaultStageDir = "./migrated/compose"

// stageComposeApp writes the translated compose file with owner-only
// permissions (it holds generated secrets) and records the deploy command.
func stageComposeApp(a *mappedApp, dir string) {
	if dir == "" {
		dir = defaultStageDir
	}
	if hasRawMagicVar(a.Compose.Yaml) {
		a.CreateError = "unresolved SERVICE_ variables remain, fix them in the staged file before deploying"
	}
	appDir := filepath.Join(dir, a.ServiceName)
	if err := os.MkdirAll(appDir, 0o700); err != nil { //nolint:gosec // name is sanitized, dir is operator-supplied
		a.CreateError = fmt.Errorf("stage compose app %q: %w", a.ServiceName, err).Error()
		return
	}
	path := filepath.Join(appDir, "compose.yaml")
	if err := os.WriteFile(path, []byte(a.Compose.Yaml), 0o600); err != nil { //nolint:gosec // name is sanitized, dir is operator-supplied
		a.CreateError = fmt.Errorf("stage compose app %q: %w", a.ServiceName, err).Error()
		return
	}
	a.Compose.StagedPath = path
	a.Compose.Deploy = fmt.Sprintf("levelrail-cli compose deploy %s -f %s", a.ServiceName, path)
	if len(a.Service.EnvLiteral) > 0 {
		envPath := filepath.Join(appDir, "stack.env")
		skipped, err := writeSecretsEnvFile(envPath, a.Service.EnvLiteral)
		if err != nil {
			a.CreateError = fmt.Errorf("stage compose app %q: %w", a.ServiceName, err).Error()
			return
		}
		a.Issues = envSkippedIssues(a.Issues, skipped)
		a.SecretsFile = envPath
		a.Compose.Deploy += fmt.Sprintf(" && levelrail-cli apps secrets set %s --env-file %s --apply", a.ServiceName, envPath)
	}
	a.Applied = a.CreateError == ""
}
