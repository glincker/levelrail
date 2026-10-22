package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploycontext"
	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ContextConfigStore is the store surface deploy contexts need. *store.DB satisfies it.
type ContextConfigStore interface {
	SaveAppContextConfig(ctx context.Context, c store.AppContextConfig) error
	DeleteAppContextConfig(ctx context.Context, appName, contextName string) (bool, error)
	ListAppContextConfigs(ctx context.Context, appName string) ([]store.AppContextConfig, error)
}

// contextConfigResource is the wire shape of one context's overrides.
type contextConfigResource struct {
	Context      string            `json:"context"`
	Env          map[string]string `json:"env"`
	BuildCommand string            `json:"build_command"`
	PublishDir   string            `json:"publish_dir"`
	BuildArgs    map[string]string `json:"build_args"`
	UpdatedAt    string            `json:"updated_at,omitempty"`
}

func contextConfigsToDeploy(rows []store.AppContextConfig) map[string]deploycontext.Config {
	out := make(map[string]deploycontext.Config, len(rows))
	for _, r := range rows {
		out[r.Context] = deploycontext.Config{Env: r.Env, BuildCommand: r.BuildCommand, Publish: r.PublishDir, BuildArgs: r.BuildArgs}
	}
	return out
}

func (rt *Router) loadContextConfigs(ctx context.Context, appName string) (map[string]deploycontext.Config, error) {
	if rt.contextConfigs == nil {
		return nil, nil
	}
	rows, err := rt.contextConfigs.ListAppContextConfigs(ctx, appName)
	if err != nil {
		return nil, err
	}
	return contextConfigsToDeploy(rows), nil
}

// applyContextConfig layers a deploy's context env and build settings onto
// svcSpec: the context first, then matching branch contexts.
func applyContextConfig(svcSpec *spec.Service, contextName, branch string, contexts map[string]deploycontext.Config) {
	for _, l := range deploycontext.Layers(contextName, branch, contexts) {
		if len(l.Config.Env) > 0 && svcSpec.Env == nil {
			svcSpec.Env = make(map[string]spec.EnvVar, len(l.Config.Env))
		}
		for k, v := range l.Config.Env {
			svcSpec.Env[k] = spec.EnvVar{Value: v}
		}
		if l.Config.BuildCommand != "" {
			svcSpec.Build.Command = l.Config.BuildCommand
		}
		if l.Config.Publish != "" {
			svcSpec.Build.Publish = l.Config.Publish
		}
		if len(l.Config.BuildArgs) > 0 && svcSpec.Build.Args == nil {
			svcSpec.Build.Args = make(map[string]string, len(l.Config.BuildArgs))
		}
		for k, v := range l.Config.BuildArgs {
			svcSpec.Build.Args[k] = v
		}
	}
	if svcSpec.Build.Command != "" {
		svcSpec.Build.Path = ""
	}
}

// handleListAppContexts handles GET /api/v1/apps/{name}/contexts.
func (rt *Router) handleListAppContexts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.contextConfigs == nil {
		writeJSON(w, http.StatusOK, []contextConfigResource{})
		return
	}
	rows, err := rt.contextConfigs.ListAppContextConfigs(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list app contexts failed", err, slog.String("name", name))
		return
	}
	out := make([]contextConfigResource, 0, len(rows))
	for _, c := range rows {
		env, _ := envredact.Map(c.Env, nil)
		out = append(out, contextConfigResource{Context: c.Context, Env: env, BuildCommand: c.BuildCommand, PublishDir: c.PublishDir, BuildArgs: c.BuildArgs, UpdatedAt: c.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetAppContext handles PUT /api/v1/apps/{name}/contexts: it replaces one context's overrides.
// Masked placeholders in env keep the stored value, so a list-edit-save round trip never writes a mask.
func (rt *Router) handleSetAppContext(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req contextConfigResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := deploycontext.Validate(req.Context); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rt.contextConfigs == nil {
		writeError(w, http.StatusNotImplemented, "deploy contexts are not configured on this control plane")
		return
	}
	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app context: load app failed", err, slog.String("name", name))
		return
	}
	buildType := spec.BuildDockerfile
	if rt.gitSources != nil {
		if gs, gerr := rt.gitSources.GetGitSource(r.Context(), name); gerr == nil && gs != nil {
			buildType = gs.BuildType
		}
	}
	cfg := deploycontext.Config{Env: req.Env, BuildCommand: req.BuildCommand, Publish: req.PublishDir, BuildArgs: req.BuildArgs}
	if err := spec.ValidateContextConfig("contexts."+req.Context, buildType, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	env := req.Env
	if rows, lerr := rt.contextConfigs.ListAppContextConfigs(r.Context(), name); lerr == nil {
		for _, row := range rows {
			if row.Context == req.Context {
				merged, merr := envredact.Merge(req.Env, row.Env)
				if merr != nil {
					writeError(w, http.StatusBadRequest, merr.Error())
					return
				}
				env = merged
			}
		}
	}
	row := store.AppContextConfig{AppName: name, Context: req.Context, Env: env, BuildCommand: req.BuildCommand, PublishDir: req.PublishDir, BuildArgs: req.BuildArgs}
	if err := rt.contextConfigs.SaveAppContextConfig(r.Context(), row); err != nil {
		rt.internalError(w, "api: set app context failed", err, slog.String("name", name), slog.String("context", req.Context))
		return
	}
	shown, _ := envredact.Map(env, nil)
	req.Env = shown
	writeJSON(w, http.StatusOK, req)
}

// handleDeleteAppContext handles DELETE /api/v1/apps/{name}/contexts?context=.
func (rt *Router) handleDeleteAppContext(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	contextName := r.URL.Query().Get("context")
	if rt.contextConfigs == nil {
		writeError(w, http.StatusNotFound, "context not found")
		return
	}
	found, err := rt.contextConfigs.DeleteAppContextConfig(r.Context(), name, contextName)
	if err != nil {
		rt.internalError(w, "api: delete app context failed", err, slog.String("name", name))
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "context not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetEffectiveConfig handles GET /api/v1/apps/{name}/effective-config?context=&branch=:
// the env and build settings a deploy in that context would use, with the source of each value.
func (rt *Router) handleGetEffectiveConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	contextName := r.URL.Query().Get("context")
	if contextName == "" {
		contextName = deploycontext.Production
	}
	branch := r.URL.Query().Get("branch")
	if contextName != deploycontext.Production && deploycontext.Validate(contextName) != nil {
		writeError(w, http.StatusBadRequest, "unknown context "+contextName)
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: effective config: load app failed", err, slog.String("name", name))
		return
	}
	in, err := rt.effectiveInputs(r.Context(), name, svc, contextName, branch)
	if err != nil {
		rt.internalError(w, "api: effective config failed", err, slog.String("name", name))
		return
	}
	eff := deploycontext.Resolve(in)
	for i, e := range eff.Env {
		if !e.Secret && envredact.SensitiveName(e.Key) {
			eff.Env[i].Value, eff.Env[i].Secret = deploycontext.MaskedValue, true
		}
	}
	writeJSON(w, http.StatusOK, eff)
}

func (rt *Router) effectiveInputs(ctx context.Context, name string, svc *store.DesiredService, contextName, branch string) (deploycontext.Inputs, error) {
	in := deploycontext.Inputs{Context: contextName, Branch: branch, AppEnv: map[string]deploycontext.EnvValue{}, LegacyPreview: svc.PreviewEnvOverrides}
	for k, v := range svc.Env {
		in.AppEnv[k] = deploycontext.EnvValue{Value: v}
	}
	for _, ref := range svc.SecretEnv {
		in.AppEnv[ref.Name] = deploycontext.EnvValue{Secret: true}
	}
	for k := range svc.VaultEnv {
		in.AppEnv[k] = deploycontext.EnvValue{Secret: true}
	}
	if rt.gitSources != nil {
		if gs, err := rt.gitSources.GetGitSource(ctx, name); err == nil && gs != nil {
			if b, berr := gs.SpecBuild(); berr == nil {
				in.AppBuild = deploycontext.Config{BuildCommand: b.Command, Publish: b.Publish, BuildArgs: b.Args}
			}
		}
	}
	var err error
	if in.Contexts, err = rt.loadContextConfigs(ctx, name); err != nil {
		return in, err
	}
	overrides, err := rt.apps.ListServiceBranchEnvOverrides(ctx, name)
	if err != nil {
		return in, err
	}
	for _, o := range overrides {
		in.BranchEnv = append(in.BranchEnv, deploycontext.BranchOverride{Pattern: o.BranchPattern, Key: o.Key, Value: o.Value, Secret: o.Secret})
	}
	return in, nil
}

func specContexts(in map[string]spec.ContextConfig) map[string]deploycontext.Config {
	out := make(map[string]deploycontext.Config, len(in))
	for k, v := range in {
		out[k] = v.DeployConfig()
	}
	return out
}
