package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/repoplan"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvImportConfirmAbove is the package count above which an import must set confirm.
const EnvImportConfirmAbove = "APP_WIZARD_IMPORT_CONFIRM_ABOVE"

const defaultImportConfirmAbove = 10

// Per-app result statuses of an import apply.
const (
	importStatusCreated           = "created"
	importStatusAlreadyExists     = "already_exists"
	importStatusSkippedDependency = "skipped_dependency"
	importStatusFailed            = "failed"
	importStatusWouldCreate       = "would_create"
)

// Error codes a dashboard can translate.
const (
	importCodeUnknownPackage    = "unknown_package"
	importCodePackageMismatch   = "package_mismatch"
	importCodeDuplicate         = "duplicate"
	importCodeInvalidName       = "invalid_name"
	importCodeInvalidPort       = "invalid_port"
	importCodeInvalidEnv        = "invalid_env"
	importCodeNameTaken         = "name_taken"
	importCodeUnsupportedBuild  = "unsupported_build"
	importCodeProtectedEnv      = "protected_environment"
	importCodeUnknownProject    = "unknown_project"
	importCodeUnknownEnv        = "unknown_environment"
	importCodeForbidden         = "forbidden"
	importCodeDependencyFailed  = "dependency_failed"
	importCodeConfirmation      = "confirmation_required"
	importCodeSourceFailed      = "git_source_failed"
	importCodeEnvironmentFailed = "environment_failed"
)

var importAppNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

type importApplyPackage struct {
	Package       string            `json:"package"`
	Name          string            `json:"name"`
	Directory     string            `json:"directory"`
	Port          int               `json:"port,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	SecretEnvKeys []string          `json:"secret_env_keys,omitempty"`
	ProjectID     string            `json:"project_id,omitempty"`
	EnvironmentID string            `json:"environment_id,omitempty"`
	Deploy        bool              `json:"deploy,omitempty"`
}

// importApplyRequest is POST /api/v1/imports/repo/apply's body. Token is a
// one-shot read token for a private repository and is never echoed back.
type importApplyRequest struct {
	RepoURL       string               `json:"repo_url"`
	Branch        string               `json:"branch,omitempty"`
	BaseDirectory string               `json:"base_directory,omitempty"`
	Token         string               `json:"token,omitempty"`
	Packages      []importApplyPackage `json:"packages"`
	DryRun        bool                 `json:"dry_run,omitempty"`
	Confirm       bool                 `json:"confirm,omitempty"`
}

type importApplyResult struct {
	Package     string `json:"package"`
	Name        string `json:"name"`
	Directory   string `json:"directory"`
	Status      string `json:"status"`
	Code        string `json:"code,omitempty"`
	Message     string `json:"message,omitempty"`
	AppID       string `json:"app_id,omitempty"`
	DeployID    string `json:"deploy_id,omitempty"`
	DeployError string `json:"deploy_error,omitempty"`
}

type importApplyResponse struct {
	RepoURL           string              `json:"repo_url"`
	Branch            string              `json:"branch"`
	DryRun            bool                `json:"dry_run"`
	NeedsConfirmation bool                `json:"needs_confirmation"`
	ConfirmAbove      int                 `json:"confirm_above"`
	Counts            map[string]int      `json:"counts"`
	Results           []importApplyResult `json:"results"`
}

// handleImportRepoApply creates one app per reviewed package. The plan is
// recomputed server side, so directories and build kinds never come from the
// client, and each app goes through the single-app create handlers.
func (rt *Router) handleImportRepoApply(w http.ResponseWriter, r *http.Request) {
	var req importApplyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDecodeBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Packages) == 0 {
		writeError(w, http.StatusBadRequest, "packages is required")
		return
	}
	plan, status, msg := rt.freshImportPlan(r, req)
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	rows := make(map[string]repoplan.Row, len(plan.Packages))
	for _, row := range plan.Packages {
		rows[row.Directory] = row
	}

	results := make([]importApplyResult, len(req.Packages))
	seenName, seenDir := map[string]bool{}, map[string]bool{}
	for i, p := range req.Packages {
		results[i] = importApplyResult{Package: p.Package, Name: strings.TrimSpace(p.Name), Directory: strings.Trim(p.Directory, "/ ")}
		code, text := rt.checkImportPackage(r, plan, rows, req.Packages[i], results[i].Name, results[i].Directory, seenName, seenDir)
		switch code {
		case "":
			results[i].Status = importStatusWouldCreate
		case importStatusAlreadyExists:
			results[i].Status = importStatusAlreadyExists
			results[i].Message = text
		default:
			results[i].Status, results[i].Code, results[i].Message = importStatusFailed, code, text
		}
	}
	rt.applyImportQuota(r.Context(), req.Packages, results)

	pending := 0
	for _, res := range results {
		if res.Status == importStatusWouldCreate {
			pending++
		}
	}
	above := envIntDefault(EnvImportConfirmAbove, defaultImportConfirmAbove)
	needs := pending > above
	if !req.DryRun && needs && !req.Confirm {
		writeCodedError(w, http.StatusConflict, importCodeConfirmation, fmt.Sprintf("this import creates %d apps, more than %d; set confirm to proceed", pending, above))
		return
	}
	if !req.DryRun {
		rt.runImportApply(r, plan, rows, req, results)
		rt.nudgeReconciler()
	}

	counts := map[string]int{}
	for _, res := range results {
		counts[res.Status]++
	}
	writeJSON(w, http.StatusOK, importApplyResponse{
		RepoURL: plan.RepoURL, Branch: plan.Branch, DryRun: req.DryRun, NeedsConfirmation: needs,
		ConfirmAbove: above, Counts: counts, Results: results,
	})
}

// freshImportPlan reuses the plan handler so apply and preview can never disagree.
func (rt *Router) freshImportPlan(r *http.Request, req importApplyRequest) (repoplan.Plan, int, string) {
	body, _ := json.Marshal(importRepoPlanRequest{RepoURL: req.RepoURL, Branch: req.Branch, BaseDirectory: req.BaseDirectory, Token: req.Token})
	status, out := rt.callImportHandler(r, rt.handleImportRepoPlan, http.MethodPost, "/api/v1/imports/repo/plan", "", body)
	if status != http.StatusOK {
		return repoplan.Plan{}, status, importErrorText(out)
	}
	var plan repoplan.Plan
	if err := json.Unmarshal(out, &plan); err != nil {
		return repoplan.Plan{}, http.StatusInternalServerError, "internal error"
	}
	return plan, http.StatusOK, ""
}

// checkImportPackage returns an empty code when the package can be created,
// importStatusAlreadyExists for an idempotent re-apply, or a failure code.
func (rt *Router) checkImportPackage(r *http.Request, plan repoplan.Plan, rows map[string]repoplan.Row, p importApplyPackage, name, dir string, seenName, seenDir map[string]bool) (string, string) {
	row, ok := rows[dir]
	switch {
	case !ok:
		return importCodeUnknownPackage, "this directory is not a package of the repository"
	case p.Package != "" && p.Package != row.PackageName:
		return importCodePackageMismatch, "the package name does not match the repository"
	case seenDir[dir] || seenName[strings.ToLower(name)]:
		return importCodeDuplicate, "this package or app name appears twice in the request"
	}
	seenDir[dir], seenName[strings.ToLower(name)] = true, true
	if name == "" || len(name) > appwizard.MaxAppNameLen || !importAppNameRe.MatchString(name) {
		return importCodeInvalidName, "use lowercase letters, digits and hyphens"
	}
	if row.BuildKind == repoplan.KindStatic {
		return importCodeUnsupportedBuild, "static sites are not imported in bulk yet; create this one from the single app flow"
	}
	if port := importPort(p, row); port < 1 || port > 65535 {
		return importCodeInvalidPort, "set a port between 1 and 65535"
	}
	if _, err := envredact.Merge(p.Env, nil); err != nil {
		return importCodeInvalidEnv, err.Error()
	}
	for key := range p.Env {
		if strings.TrimSpace(key) == "" {
			return importCodeInvalidEnv, "environment variable names must not be empty"
		}
	}
	if code, text := rt.checkImportScope(r, p); code != "" {
		return code, text
	}
	ctx := r.Context()
	if _, err := rt.apps.GetDesiredService(ctx, name); err == nil {
		gs, gerr := rt.gitSources.GetGitSource(ctx, name)
		if gerr == nil && gs != nil && sameRepoURL(gs.RepoURL, plan.RepoURL) && strings.Trim(gs.BaseDirectory, "/ ") == dir {
			return importStatusAlreadyExists, "an app with this name already tracks this package"
		}
		return importCodeNameTaken, "an app with this name already exists"
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		return importCodeNameTaken, "could not check the app name"
	}
	return "", ""
}

func importPort(p importApplyPackage, row repoplan.Row) int {
	if p.Port != 0 {
		return p.Port
	}
	if row.Port != nil {
		return *row.Port
	}
	return 0
}

// checkImportScope covers project, environment and ability rules the
// single-app path enforces through its routes.
func (rt *Router) checkImportScope(r *http.Request, p importApplyPackage) (string, string) {
	ctx := r.Context()
	if p.ProjectID != "" {
		if err := rt.validateProjectID(ctx, p.ProjectID); err != nil {
			return importCodeUnknownProject, "unknown project_id"
		}
	}
	if p.EnvironmentID != "" {
		env, err := rt.environments.GetEnvironment(ctx, p.EnvironmentID)
		if err != nil || (p.ProjectID != "" && env.ProjectID != "" && env.ProjectID != p.ProjectID) {
			return importCodeUnknownEnv, "unknown environment_id for this project"
		}
		if env.Protected {
			return importCodeProtectedEnv, "this environment is protected; create the app elsewhere and move it with approval"
		}
	}
	name := strings.TrimSpace(p.Name)
	checks := []struct {
		ability string
		refs    []resourceRef
	}{
		{AbilityWriteSensitive, []resourceRef{appRef(name)}},
		{AbilityWrite, []resourceRef{projectRef(p.ProjectID), environmentRef(p.EnvironmentID)}},
	}
	if p.Deploy {
		checks = append(checks, struct {
			ability string
			refs    []resourceRef
		}{AbilityDeploy, []resourceRef{appRef(name)}})
	}
	for _, c := range checks {
		denied, err := rt.firstDeniedRef(r, c.ability, c.refs)
		if err != nil {
			rt.logger.Error("api: import apply: authorize failed", slog.String("error", err.Error()), slog.String("name", name))
			return importCodeForbidden, "could not verify permissions"
		}
		if denied != nil {
			return importCodeForbidden, "your account lacks the required ability for this app, project or environment"
		}
	}
	return "", ""
}

// applyImportQuota fails the apps that would exceed an app quota, counting
// the earlier would_create rows of the same project as already used.
func (rt *Router) applyImportQuota(ctx context.Context, pkgs []importApplyPackage, results []importApplyResult) {
	planned := map[string]int{}
	for i := range results {
		if results[i].Status != importStatusWouldCreate {
			continue
		}
		project := pkgs[i].ProjectID
		planned[project]++
		release, denial, err := rt.admitQuota(ctx, project, store.ResourceOwner{}, quota.Amounts{Apps: int64(planned[project])})
		release()
		if err != nil {
			rt.logger.Error("api: import apply: quota check failed", slog.String("error", err.Error()))
			continue
		}
		if denial != nil {
			planned[project]--
			results[i].Status, results[i].Code, results[i].Message = importStatusFailed, quotaExceededCode, denial.message()
		}
	}
}

// runImportApply creates apps in dependency order. A failed app skips its
// dependents; independent apps carry on.
func (rt *Router) runImportApply(r *http.Request, plan repoplan.Plan, rows map[string]repoplan.Row, req importApplyRequest, results []importApplyResult) {
	byDir := map[string]int{}
	for i := range results {
		byDir[results[i].Directory] = i
	}
	for _, i := range importOrder(results, rows) {
		res := &results[i]
		if res.Status != importStatusWouldCreate {
			continue
		}
		for _, dep := range rows[res.Directory].DependsOn {
			j, ok := byDir[dep]
			if ok && results[j].Status != importStatusCreated && results[j].Status != importStatusAlreadyExists {
				res.Status, res.Code, res.Message = importStatusSkippedDependency, importCodeDependencyFailed, "skipped because "+results[j].Name+" was not created"
				break
			}
		}
		if res.Status != importStatusWouldCreate {
			continue
		}
		rt.createImportApp(r, plan, rows[res.Directory], req, req.Packages[i], res)
	}
}

// importOrder lists result indexes dependencies first; cycles break by directory.
func importOrder(results []importApplyResult, rows map[string]repoplan.Row) []int {
	byDir := map[string]int{}
	for i := range results {
		byDir[results[i].Directory] = i
	}
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	done := map[string]bool{}
	visiting := map[string]bool{}
	var out []int
	var visit func(d string)
	visit = func(d string) {
		if done[d] || visiting[d] {
			return
		}
		visiting[d] = true
		for _, dep := range rows[d].DependsOn {
			if _, ok := byDir[dep]; ok {
				visit(dep)
			}
		}
		visiting[d] = false
		done[d] = true
		out = append(out, byDir[d])
	}
	for _, d := range dirs {
		visit(d)
	}
	return out
}

func (rt *Router) createImportApp(r *http.Request, plan repoplan.Plan, row repoplan.Row, req importApplyRequest, p importApplyPackage, res *importApplyResult) {
	env, secretKeys, secretValues := map[string]string{}, []string{}, map[string]string{}
	declared := map[string]bool{}
	for _, k := range p.SecretEnvKeys {
		declared[k] = true
	}
	for k, v := range p.Env {
		switch {
		case v == "":
		case declared[k]:
			secretKeys = append(secretKeys, k)
			secretValues[k] = v
		default:
			env[k] = v
		}
	}
	sort.Strings(secretKeys)
	app := appResource{Name: res.Name, Image: res.Name + spec.PendingImageTag, Port: importPort(p, row), ProjectID: p.ProjectID}
	if len(env) > 0 {
		app.Env = env
	}
	if len(secretValues) > 0 {
		app.SecretEnv, app.Secrets = secretKeys, secretValues
	}
	body, _ := json.Marshal(app)
	status, out := rt.callImportHandler(r, rt.handleCreateApp, http.MethodPost, "/api/v1/apps", "", body)
	if status != http.StatusCreated {
		res.Status, res.Code, res.Message = importStatusFailed, importErrorCode(out, status), importErrorText(out)
		return
	}
	var created struct {
		AppID string `json:"app_id"`
	}
	_ = json.Unmarshal(out, &created)
	res.Status, res.AppID = importStatusCreated, created.AppID

	buildType, buildPath := spec.BuildRailpack, ""
	if row.BuildKind == repoplan.KindDockerfile {
		buildType, buildPath = spec.BuildDockerfile, row.Dockerfile
	}
	branch := plan.Branch
	body, _ = json.Marshal(setGitSourceRequest{RepoURL: plan.RepoURL, Branch: branch, BuildType: buildType, BuildPath: buildPath, BaseDirectory: row.Directory, Token: req.Token})
	if status, out = rt.callImportHandler(r, rt.handleSetGitSource, http.MethodPut, "/api/v1/apps/"+res.Name+"/git-source", res.Name, body); status != http.StatusOK && status != http.StatusCreated {
		res.Status, res.Code, res.Message = importStatusFailed, importCodeSourceFailed, "the app was created but connecting the repository failed: "+importErrorText(out)
		return
	}
	if p.EnvironmentID != "" {
		body, _ = json.Marshal(setAppEnvironmentRequest{EnvironmentID: p.EnvironmentID})
		if status, out = rt.callImportHandler(r, rt.handleSetAppEnvironment, http.MethodPut, "/api/v1/apps/"+res.Name+"/environment", res.Name, body); status != http.StatusOK {
			res.Status, res.Code, res.Message = importStatusFailed, importCodeEnvironmentFailed, "the app was created but assigning the environment failed: "+importErrorText(out)
			return
		}
	}
	if !p.Deploy {
		return
	}
	build := triggerBuildBuildInput{Type: buildType, Path: buildPath, BaseDirectory: row.Directory}
	body, _ = json.Marshal(triggerBuildRequest{RepoURL: plan.RepoURL, Ref: branch, Build: build})
	status, out = rt.callImportHandler(r, rt.handleTriggerBuild, http.MethodPost, "/api/v1/apps/"+res.Name+"/builds", res.Name, body)
	if status >= 200 && status < 300 {
		var built triggerBuildResponse
		_ = json.Unmarshal(out, &built)
		res.DeployID = built.ID
		return
	}
	res.DeployError = importErrorText(out)
}

// callImportHandler runs a single-app handler in process under the caller's
// own request context, so its validation, quotas and audit apply unchanged.
func (rt *Router) callImportHandler(r *http.Request, h http.HandlerFunc, method, target, name string, body []byte) (int, []byte) {
	inner := httptest.NewRequestWithContext(r.Context(), method, target, bytes.NewReader(body))
	inner.Header = r.Header.Clone()
	inner.Header.Del("Content-Length")
	inner.Header.Del("Idempotency-Key")
	inner.Header.Set("Content-Type", "application/json")
	inner.RemoteAddr, inner.Host, inner.TLS = r.RemoteAddr, r.Host, r.TLS
	if name != "" {
		inner.SetPathValue("name", name)
	}
	rec := httptest.NewRecorder()
	h(rec, inner)
	return rec.Code, rec.Body.Bytes()
}

func importErrorText(body []byte) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	switch {
	case e.Error != "":
		return e.Error
	case e.Message != "":
		return e.Message
	}
	return "request failed"
}

func importErrorCode(body []byte, status int) string {
	var e struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Code != "" {
		return e.Code
	}
	return errorCodeForStatus(status)
}
