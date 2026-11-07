package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// AuditActionProjectClone is the audit event for an applied project clone;
	// its detail holds counts only, never a value.
	AuditActionProjectClone = "project.clone"

	envProjectCloneMaxApps     = "APP_PROJECT_CLONE_MAX_APPS"
	envProjectClonePlaceholder = "APP_PROJECT_CLONE_PLACEHOLDER_DOMAIN"
	defaultProjectCloneMaxApps = 50
	defaultCloneDomainSuffix   = "clone.invalid"

	cloneDomainsNone        = "none"
	cloneDomainsPlaceholder = "placeholder"
)

type projectCloneInclude struct {
	EnvVars  bool   `json:"env_vars"`
	Domains  string `json:"domains"`
	Settings bool   `json:"settings"`
}

type projectCloneRequest struct {
	Name        string              `json:"name"`
	Environment string              `json:"environment,omitempty"`
	Include     projectCloneInclude `json:"include"`
	DryRun      bool                `json:"dry_run"`
}

type projectCloneApp struct {
	Source  string   `json:"source"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	EnvVars int      `json:"env_vars"`
}

type projectCloneSecretKey struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
}

type projectCloneSkip struct {
	Scope  string `json:"scope"`
	Item   string `json:"item"`
	Reason string `json:"reason"`
}

type projectCloneProject struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// projectClonePlan is both the dry-run answer and the apply receipt.
type projectClonePlan struct {
	DryRun      bool                    `json:"dry_run"`
	Applied     bool                    `json:"applied"`
	Source      projectCloneProject     `json:"source"`
	Project     projectCloneProject     `json:"project"`
	Environment string                  `json:"environment,omitempty"`
	Apps        []projectCloneApp       `json:"apps"`
	EnvVarCount int                     `json:"env_var_count"`
	SecretKeys  []projectCloneSecretKey `json:"secret_keys_needing_values"`
	Skipped     []projectCloneSkip      `json:"skipped"`

	services       []store.DesiredService
	projectEnv     map[string]string
	projectSecrets map[string]bool
	envEnv         map[string]string
	srcKind        string
}

// handleCloneProject handles POST /api/v1/projects/{id}/clone: copies the
// configuration of a project, or one environment of it, into a new project.
// Secret values, volume data, database data and live domains never travel.
func (rt *Router) handleCloneProject(w http.ResponseWriter, r *http.Request) {
	var req projectCloneRequest
	if !decodeReq(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Include.Domains == "" {
		req.Include.Domains = cloneDomainsNone
	}
	if req.Include.Domains != cloneDomainsNone && req.Include.Domains != cloneDomainsPlaceholder {
		writeError(w, http.StatusBadRequest, "include.domains must be none or placeholder")
		return
	}
	src, ok := rt.exportProject(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	plan, status, msg := rt.buildProjectClonePlan(r, src, req)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	if req.DryRun {
		plan.DryRun = true
		writeJSON(w, http.StatusOK, plan)
		return
	}
	if err := rt.applyProjectClone(r, plan); err != nil {
		rt.internalError(w, "api: clone project: apply failed", err, slog.String("id", src.ID))
		return
	}
	plan.Applied = true
	rt.auditProjectClone(r, src.ID, plan)
	writeJSON(w, http.StatusCreated, plan)
}

// buildProjectClonePlan reads the source and decides, without writing, what
// is created and what is skipped. A non-zero status is the refusal.
func (rt *Router) buildProjectClonePlan(r *http.Request, src store.Project, req projectCloneRequest) (*projectClonePlan, int, string) {
	ctx := r.Context()
	svcs, envName, status, msg := rt.cloneSourceServices(r, src, req.Environment)
	if status != 0 {
		return nil, status, msg
	}
	if len(svcs) == 0 {
		return nil, http.StatusNotFound, "nothing to clone: no apps in the selected scope"
	}
	if limit := envIntDefault(envProjectCloneMaxApps, defaultProjectCloneMaxApps); len(svcs) > limit {
		return nil, http.StatusUnprocessableEntity, fmt.Sprintf("project has %d apps, the clone limit is %d", len(svcs), limit)
	}
	names := make([]string, len(svcs))
	for i := range svcs {
		names[i] = svcs[i].Name
	}
	denied, err := rt.firstDeniedRef(r, AbilityRead, append(appRefs(names), projectRef(src.ID)))
	if err != nil {
		rt.logger.Warn("api: clone project: authz failed", slog.String("id", src.ID), slog.String("error", err.Error()))
		return nil, http.StatusInternalServerError, "internal error"
	}
	if denied != nil {
		return nil, http.StatusNotFound, "project not found"
	}
	newName, err := rt.freeProjectName(r, req.Name)
	if err != nil {
		rt.logger.Warn("api: clone project: list projects failed", slog.String("error", err.Error()))
		return nil, http.StatusInternalServerError, "internal error"
	}
	plan := &projectClonePlan{
		Source:      projectCloneProject{ID: src.ID, Name: src.Name},
		Project:     projectCloneProject{Name: newName},
		Environment: envName,
		Apps:        []projectCloneApp{},
		SecretKeys:  []projectCloneSecretKey{},
		Skipped:     []projectCloneSkip{},
	}
	if newName != req.Name {
		plan.Skipped = append(plan.Skipped, projectCloneSkip{Scope: "project", Item: req.Name, Reason: "name taken, using " + newName})
	}
	taken := map[string]bool{}
	rename := map[string]string{}
	for i := range svcs {
		n, err := rt.freeAppName(r, svcs[i].Name, taken)
		if err != nil {
			rt.logger.Warn("api: clone project: name check failed", slog.String("error", err.Error()))
			return nil, http.StatusInternalServerError, "internal error"
		}
		rename[svcs[i].Name] = n
	}
	suffix := strings.TrimSpace(os.Getenv(envProjectClonePlaceholder))
	if suffix == "" {
		suffix = defaultCloneDomainSuffix
	}
	for i := range svcs {
		app, clone := cloneService(svcs[i], rename, req.Include, suffix, plan)
		plan.Apps = append(plan.Apps, app)
		plan.services = append(plan.services, clone)
	}
	if err := rt.cloneProjectScope(ctx, src, envName != "", req.Include.EnvVars, plan); err != nil {
		rt.logger.Warn("api: clone project: read env failed", slog.String("id", src.ID), slog.String("error", err.Error()))
		return nil, http.StatusInternalServerError, "internal error"
	}
	sort.Slice(plan.SecretKeys, func(i, j int) bool {
		if plan.SecretKeys[i].Scope != plan.SecretKeys[j].Scope {
			return plan.SecretKeys[i].Scope < plan.SecretKeys[j].Scope
		}
		return plan.SecretKeys[i].Key < plan.SecretKeys[j].Key
	})
	return plan, 0, ""
}

// cloneSourceServices returns the apps in scope and the resolved source
// environment name ("" when the whole project is cloned).
func (rt *Router) cloneSourceServices(r *http.Request, src store.Project, envRef string) ([]store.DesiredService, string, int, string) {
	ctx := r.Context()
	if envRef == "" {
		svcs, err := rt.apps.ListDesiredServicesByProject(ctx, src.ID)
		if err != nil {
			rt.logger.Warn("api: clone project: list apps failed", slog.String("id", src.ID), slog.String("error", err.Error()))
			return nil, "", http.StatusInternalServerError, "internal error"
		}
		return svcs, "", 0, ""
	}
	envs, err := rt.environments.ListEnvironmentsByProject(ctx, src.ID)
	if err != nil {
		rt.logger.Warn("api: clone project: list environments failed", slog.String("id", src.ID), slog.String("error", err.Error()))
		return nil, "", http.StatusInternalServerError, "internal error"
	}
	for _, e := range envs {
		if e.ID == envRef || e.Name == envRef {
			svcs, lerr := rt.apps.ListDesiredServicesByEnvironment(ctx, e.ID)
			if lerr != nil {
				rt.logger.Warn("api: clone project: list env apps failed", slog.String("env", e.ID), slog.String("error", lerr.Error()))
				return nil, "", http.StatusInternalServerError, "internal error"
			}
			return svcs, e.Name, 0, ""
		}
	}
	return nil, "", http.StatusNotFound, "environment not found in project"
}

func (rt *Router) freeProjectName(r *http.Request, want string) (string, error) {
	all, err := rt.projects.ListProjects(r.Context())
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(all))
	for _, p := range all {
		used[p.Name] = true
	}
	return firstFreeName(want, "-", func(n string) bool { return used[n] }), nil
}

func (rt *Router) freeAppName(r *http.Request, base string, taken map[string]bool) (string, error) {
	var lookupErr error
	n := firstFreeName(base+"-copy", "-", func(c string) bool {
		if taken[c] {
			return true
		}
		svc, err := rt.apps.GetDesiredService(r.Context(), c)
		if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			lookupErr = err
			return false
		}
		return err == nil && svc != nil
	})
	if lookupErr != nil {
		return "", lookupErr
	}
	taken[n] = true
	return n, nil
}

// firstFreeName returns want, or want plus sep and the first counter from 2
// that used() does not report taken.
func firstFreeName(want, sep string, used func(string) bool) string {
	if !used(want) {
		return want
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s%s%d", want, sep, i)
		if !used(c) {
			return c
		}
	}
}

// cloneService builds the new desired service and records, on the plan,
// every secret key that needs a value and every item left behind.
func cloneService(src store.DesiredService, rename map[string]string, inc projectCloneInclude, suffix string, plan *projectClonePlan) (projectCloneApp, store.DesiredService) {
	name := rename[src.Name]
	out := store.DesiredService{Name: name, Image: src.Image, Port: src.Port}
	if inc.Settings {
		out = src
		out.Name = name
		out.ImageID, out.ImageIDRef = "", ""
		out.RestartNonce, out.EnvDirty = "", false
		out.ProjectID, out.EnvironmentID = "", ""
		out.HostPort, out.PublishedPorts = nil, nil
		out.PreviewEnvOverrides, out.NextCache, out.NextImageCache = nil, "", ""
		out.AppID, out.Incarnation, out.MaintenanceHold = "", "", ""
		out.IsTrial = false
		out.DependsOn, out.DependsOnHealthy = remapNames(src.DependsOn, rename), remapNames(src.DependsOnHealthy, rename)
	}
	skip := func(item, reason string) {
		plan.Skipped = append(plan.Skipped, projectCloneSkip{Scope: "app/" + src.Name, Item: item, Reason: reason})
	}
	if !inc.Settings {
		skip("settings", "not requested: image and port only")
	} else {
		for _, v := range []struct {
			item string
			has  bool
		}{
			{"volumes", len(src.Volumes) > 0}, {"bind mounts", len(src.BindMounts) > 0},
			{"files", len(src.Files) > 0}, {"log drain", src.LogDrain != nil},
		} {
			if v.has {
				skip(v.item, "data and credentials never travel with a clone")
			}
		}
		out.Volumes, out.BindMounts, out.Files, out.LogDrain = nil, nil, nil, nil
	}
	out.DatabaseEnv, out.VaultEnv, out.DatabaseAttachment, out.StorageTargetID = nil, nil, nil, ""
	if len(src.DatabaseEnv) > 0 || len(src.VaultEnv) > 0 || src.DatabaseAttachment != nil {
		skip("database and vault links", "they point at the source resources")
	}
	if src.HostPort != nil || len(src.PublishedPorts) > 0 {
		skip("host ports", "would collide with the source app")
	}
	secrets := map[string]bool{}
	for _, s := range src.SecretEnv {
		secrets[s.Name] = true
	}
	out.SecretEnv = append([]store.SecretEnvRef(nil), src.SecretEnv...)
	out.Env = map[string]string{}
	copied := 0
	for k, v := range src.Env {
		switch {
		case !inc.EnvVars:
		case envredact.SensitiveName(k) || envredact.SensitiveValue(k, v):
			if !secrets[k] {
				out.SecretEnv = append(out.SecretEnv, store.SecretEnvRef{Name: k})
				secrets[k] = true
			}
		default:
			out.Env[k] = v
			copied++
		}
	}
	if !inc.EnvVars {
		skip("env vars", "not requested: secret keys still listed")
	}
	for k := range secrets {
		plan.SecretKeys = append(plan.SecretKeys, projectCloneSecretKey{Scope: "app/" + name, Key: k})
	}
	out.Domains = nil
	if inc.Domains == cloneDomainsPlaceholder && len(src.Domains) > 0 {
		out.Domains = []string{name + "." + suffix}
	} else if len(src.Domains) > 0 {
		skip("domains", "live domains are never copied")
	}
	return projectCloneApp{Source: src.Name, Name: name, Domains: append([]string{}, out.Domains...), EnvVars: copied}, out
}

func remapNames(in []string, rename map[string]string) []string {
	var out []string
	for _, n := range in {
		if m, ok := rename[n]; ok {
			out = append(out, m)
		}
	}
	return out
}
