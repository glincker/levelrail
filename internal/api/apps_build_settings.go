package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionAppBuildSettingsChange is the audit event for a build settings save.
const AuditActionAppBuildSettingsChange = "app.build_settings.change"

// BuildSettingsStore is the store surface the build settings routes need. *store.DB satisfies it.
type BuildSettingsStore interface {
	GetAppBuildSettings(ctx context.Context, appName string) (store.AppBuildSettings, bool, error)
	SaveAppBuildSettings(ctx context.Context, s store.AppBuildSettings) error
}

// appBuildSettingsResource is the wire shape. Builder and Dockerfile path
// live on the git source; the rest is app_build_settings.
type appBuildSettingsResource struct {
	Builder        string   `json:"builder"`
	DockerfilePath string   `json:"dockerfile_path"`
	Target         string   `json:"target"`
	NoCache        bool     `json:"no_cache"`
	Provider       string   `json:"provider"`
	HasGitSource   bool     `json:"has_git_source"`
	Providers      []string `json:"providers"`
	MaxValueBytes  int      `json:"max_value_bytes"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

// setAppBuildSettingsRequest uses pointers so an omitted field keeps its value.
type setAppBuildSettingsRequest struct {
	Builder        *string `json:"builder,omitempty"`
	DockerfilePath *string `json:"dockerfile_path,omitempty"`
	Target         *string `json:"target,omitempty"`
	NoCache        *bool   `json:"no_cache,omitempty"`
	Provider       *string `json:"provider,omitempty"`
}

func buildSettingValueMax() int {
	return runtimesettings.ClampLimit(runtimesettings.KeyBuildSettingValueMaxBytes, 0)
}

func (rt *Router) loadAppBuildSettings(ctx context.Context, name string) (appBuildSettingsResource, error) {
	out := appBuildSettingsResource{Providers: build.RailpackProviders(), MaxValueBytes: buildSettingValueMax()}
	if rt.gitSources != nil {
		gs, err := rt.gitSources.GetGitSource(ctx, name)
		if err != nil && !errors.Is(err, store.ErrGitSourceNotFound) {
			return out, err
		}
		if err == nil && gs != nil {
			out.HasGitSource, out.Builder, out.DockerfilePath = true, gs.BuildType, gs.BuildPath
		}
	}
	if rt.buildSettings != nil {
		s, _, err := rt.buildSettings.GetAppBuildSettings(ctx, name)
		if err != nil {
			return out, err
		}
		out.Target, out.NoCache, out.Provider, out.UpdatedAt = s.Target, s.NoCache, s.Provider, s.UpdatedAt
	}
	return out, nil
}

// requireApp writes the uniform 404 and reports whether the app exists.
func (rt *Router) requireBuildSettingsApp(w http.ResponseWriter, r *http.Request, name string) bool {
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return false
	} else if err != nil {
		rt.internalError(w, "api: build settings: load app failed", err, slog.String("name", name))
		return false
	}
	return true
}

// handleGetAppBuildSettings handles GET /api/v1/apps/{name}/build-settings.
func (rt *Router) handleGetAppBuildSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireBuildSettingsApp(w, r, name) {
		return
	}
	out, err := rt.loadAppBuildSettings(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get build settings failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetAppBuildSettings handles PUT /api/v1/apps/{name}/build-settings.
// It only stores settings; the next build uses them, no deploy starts here.
// Concurrent saves are last writer wins, like PUT git-source/build.
func (rt *Router) handleSetAppBuildSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := withRequestOrigin(r.Context(), r)
	var req setAppBuildSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if rt.buildSettings == nil {
		writeError(w, http.StatusNotImplemented, "build settings are not configured on this control plane")
		return
	}
	if !rt.requireBuildSettingsApp(w, r, name) {
		return
	}
	// Builder and Dockerfile path are git source fields, gated like PUT git-source/build.
	sourceFields := req.Builder != nil || req.DockerfilePath != nil
	if sourceFields && !rt.callerHasAbility(r, AbilityWriteSensitive) {
		writeError(w, http.StatusForbidden, "changing the builder or Dockerfile path needs the write:sensitive ability")
		return
	}
	maxLen := buildSettingValueMax()
	for _, v := range []*string{req.Builder, req.DockerfilePath, req.Target, req.Provider} {
		if v != nil && len(*v) > maxLen {
			writeError(w, http.StatusBadRequest, "a build setting value is too long")
			return
		}
	}
	cur, err := rt.loadAppBuildSettings(ctx, name)
	if err != nil {
		rt.internalError(w, "api: set build settings: load failed", err, slog.String("name", name))
		return
	}
	next, sourceChanged, settingsChanged := mergeBuildSettings(cur, req)
	if err := build.ValidateSettings(build.Settings{Target: next.Target, NoCache: next.NoCache, Provider: next.Provider}, maxLen); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if (len(settingsChanged) > 0 || len(sourceChanged) > 0) && rt.deploysFromServicesMap(ctx, name) {
		writeError(w, http.StatusConflict, "this app deploys from a services map, which does not apply stored build settings; set them in that map instead")
		return
	}
	if len(sourceChanged) > 0 {
		if !rt.saveBuildSettingsSource(ctx, w, name, next) {
			return
		}
		rt.auditAppBuildSettings(r, name, sourceChanged)
	}
	if len(settingsChanged) > 0 {
		row := store.AppBuildSettings{AppName: name, Target: next.Target, NoCache: next.NoCache, Provider: next.Provider}
		if err := rt.buildSettings.SaveAppBuildSettings(ctx, row); errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		} else if err != nil {
			rt.internalError(w, "api: set build settings failed", err, slog.String("name", name))
			return
		}
		rt.auditAppBuildSettings(r, name, settingsChanged)
	}
	out, err := rt.loadAppBuildSettings(ctx, name)
	if err != nil {
		rt.internalError(w, "api: set build settings: reload failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// mergeBuildSettings applies the present request fields and names the changed
// keys, split by where they are stored.
func mergeBuildSettings(cur appBuildSettingsResource, req setAppBuildSettingsRequest) (next appBuildSettingsResource, source, settings []string) {
	next = cur
	pick := func(key string, in *string, dst *string, changed *[]string) {
		if in != nil && strings.TrimSpace(*in) != *dst {
			*dst = strings.TrimSpace(*in)
			*changed = append(*changed, key)
		}
	}
	pick("builder", req.Builder, &next.Builder, &source)
	pick("dockerfile_path", req.DockerfilePath, &next.DockerfilePath, &source)
	pick("target", req.Target, &next.Target, &settings)
	pick("provider", req.Provider, &next.Provider, &settings)
	if req.NoCache != nil && *req.NoCache != next.NoCache {
		next.NoCache = *req.NoCache
		settings = append(settings, "no_cache")
	}
	return next, source, settings
}

func (rt *Router) deploysFromServicesMap(ctx context.Context, name string) bool {
	if rt.gitSources == nil {
		return false
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	return err == nil && gs != nil && len(gs.Services) > 0
}

// saveBuildSettingsSource writes builder and Dockerfile path through the git
// source, with the same checks PUT git-source/build applies.
func (rt *Router) saveBuildSettingsSource(ctx context.Context, w http.ResponseWriter, name string, next appBuildSettingsResource) bool {
	if rt.gitSources == nil || !next.HasGitSource {
		writeError(w, http.StatusConflict, "connect a git source before setting the builder or Dockerfile path")
		return false
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil {
		rt.internalError(w, "api: set build settings: load git source failed", err, slog.String("name", name))
		return false
	}
	buildType, err := normalizeGitSourceBuildType(next.Builder)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	base, p, err := validateGitSourceBuildPaths(buildType, gs.BaseDirectory, next.DockerfilePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if err := validateGitSourceStaticBuild(name, buildType, p, gs.BuildCommand, gs.PublishDir); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if err := rt.gitSources.SetGitSourceBuild(ctx, name, buildType, p, base, gs.BuildCommand, gs.PublishDir); err != nil {
		rt.internalError(w, "api: set build settings: save git source failed", err, slog.String("name", name))
		return false
	}
	if buildType != spec.BuildStatic && gs.StaticSPA {
		if err := rt.gitSources.SetGitSourceStaticSPA(ctx, name, false); err != nil {
			rt.internalError(w, "api: set build settings: clear spa failed", err, slog.String("name", name))
			return false
		}
	}
	return true
}

// auditAppBuildSettings records which keys changed, never their values.
func (rt *Router) auditAppBuildSettings(r *http.Request, app string, changed []string) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: build settings audit id failed", slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]any{"app": app, "changed": changed})
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPut, Path: "/api/v1/apps/" + app + "/build-settings", StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionAppBuildSettingsChange,
		Detail:     string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: build settings audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}

type heldBuildSettingsKey struct{}

// withHeldBuildSettings makes a replay build with the settings snapshotted at hold time.
func withHeldBuildSettings(ctx context.Context, s *deploy.HeldBuildSettings) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, heldBuildSettingsKey{}, *s)
}

// snapshotBuildSettings is what a protected hold stores; never nil, so a
// replay never falls back to settings changed after the hold.
func (rt *Router) snapshotBuildSettings(ctx context.Context, name string) *deploy.HeldBuildSettings {
	snap := &deploy.HeldBuildSettings{}
	if rt.buildSettings == nil {
		return snap
	}
	if s, found, err := rt.buildSettings.GetAppBuildSettings(ctx, name); err == nil && found {
		snap.Target, snap.NoCache, snap.Provider = s.Target, s.NoCache, s.Provider
	}
	return snap
}

// applyAppBuildSettings layers the stored (or held) settings onto a deploy request.
// A load failure keeps the request as is so a build never stalls on it.
func (rt *Router) applyAppBuildSettings(ctx context.Context, name string, req *deploy.Request) {
	if held, ok := ctx.Value(heldBuildSettingsKey{}).(deploy.HeldBuildSettings); ok {
		applyStoredBuildSettings(store.AppBuildSettings{Target: held.Target, NoCache: held.NoCache, Provider: held.Provider}, req)
		return
	}
	if rt.buildSettings == nil {
		return
	}
	s, found, err := rt.buildSettings.GetAppBuildSettings(ctx, name)
	if err != nil {
		rt.logger.Warn("api: load build settings failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	if found {
		applyStoredBuildSettings(s, req)
	}
}

func applyStoredBuildSettings(s store.AppBuildSettings, req *deploy.Request) {
	if req.Target == "" {
		req.Target = s.Target
	}
	req.NoCache = req.NoCache || s.NoCache
	if req.RailpackProvider == "" {
		req.RailpackProvider = s.Provider
	}
}
