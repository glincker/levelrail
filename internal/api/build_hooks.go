package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BuildHookStore is the store surface for per-app incoming build hooks.
type BuildHookStore interface {
	CreateBuildHook(ctx context.Context, h store.BuildHook) error
	ListBuildHooks(ctx context.Context, serviceName string) ([]store.BuildHook, error)
	GetBuildHook(ctx context.Context, id string) (store.BuildHook, error)
	SetBuildHookTokenHash(ctx context.Context, serviceName, id, hash string) error
	DeleteBuildHook(ctx context.Context, serviceName, id string) error
	RecordBuildHookTrigger(ctx context.Context, id, result string, at time.Time) error
	RecordBuildHookResult(ctx context.Context, id, result string) error
}

const (
	envBuildHooksPerApp = "APP_BUILD_HOOKS_MAX_PER_APP"
	buildHooksPerApp    = 20
	buildHookIDPrefix   = "bh_"
	buildHookURLPrefix  = "/api/v1/hooks/deploy/"
)

var buildHookNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$`)

type buildHookResource struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Branch          string     `json:"branch,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	LastResult      string     `json:"last_result,omitempty"`
	TriggerCount    int        `json:"trigger_count"`
	// Path and Token are set only on create and rotate, the one time the secret is visible.
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

type createBuildHookRequest struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
}

func toBuildHookResource(h store.BuildHook) buildHookResource {
	return buildHookResource{ID: h.ID, Name: h.Name, Branch: h.Branch, CreatedAt: h.CreatedAt,
		LastTriggeredAt: h.LastTriggeredAt, LastResult: h.LastResult, TriggerCount: h.TriggerCount}
}

func (rt *Router) buildHookApp(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: build hooks: load app", err, slog.String("name", name))
		return "", false
	}
	return name, true
}

func (rt *Router) handleListBuildHooks(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	hooks, err := rt.buildHooks.ListBuildHooks(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list build hooks", err, slog.String("name", name))
		return
	}
	out := make([]buildHookResource, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, toBuildHookResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateBuildHook mints a hook; the secret URL is returned once and only its hash is kept.
func (rt *Router) handleCreateBuildHook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	var req createBuildHookRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name, req.Branch = strings.TrimSpace(req.Name), strings.TrimSpace(req.Branch)
	if !buildHookNameRE.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be 1 to 63 characters: letters, digits, spaces, dot, dash, underscore")
		return
	}
	if strings.ContainsAny(req.Branch, " \t\r\n") || len(req.Branch) > 255 {
		writeError(w, http.StatusBadRequest, "invalid branch")
		return
	}
	existing, err := rt.buildHooks.ListBuildHooks(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: create build hook: list", err, slog.String("name", name))
		return
	}
	if len(existing) >= envInt(envBuildHooksPerApp, buildHooksPerApp) {
		writeError(w, http.StatusConflict, "build hook limit reached for this app")
		return
	}
	id, err := randomOpaqueID(buildHookIDPrefix)
	if err != nil {
		rt.internalError(w, "api: create build hook: mint id", err)
		return
	}
	token, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: create build hook: mint token", err)
		return
	}
	h := store.BuildHook{ID: id, ServiceName: name, Name: req.Name, Branch: req.Branch, TokenHash: hashToken(token), CreatedAt: time.Now().UTC()}
	if err := rt.buildHooks.CreateBuildHook(r.Context(), h); errors.Is(err, store.ErrBuildHookExists) {
		writeError(w, http.StatusConflict, "a build hook with that name already exists")
		return
	} else if err != nil {
		rt.internalError(w, "api: create build hook", err, slog.String("name", name))
		return
	}
	res := toBuildHookResource(h)
	res.Token, res.Path = token, buildHookURLPrefix+id+"/"+token
	writeJSON(w, http.StatusCreated, res)
}

// handleRotateBuildHook swaps the secret; the old URL stops working at once.
func (rt *Router) handleRotateBuildHook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	token, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: rotate build hook: mint token", err)
		return
	}
	if err := rt.buildHooks.SetBuildHookTokenHash(r.Context(), name, id, hashToken(token)); errors.Is(err, store.ErrBuildHookNotFound) {
		writeError(w, http.StatusNotFound, "build hook not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: rotate build hook", err, slog.String("name", name))
		return
	}
	h, err := rt.buildHooks.GetBuildHook(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: rotate build hook: reload", err, slog.String("name", name))
		return
	}
	res := toBuildHookResource(h)
	res.Token, res.Path = token, buildHookURLPrefix+id+"/"+token
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleRevokeBuildHook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	if err := rt.buildHooks.DeleteBuildHook(r.Context(), name, r.PathValue("id")); errors.Is(err, store.ErrBuildHookNotFound) {
		writeError(w, http.StatusNotFound, "build hook not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: revoke build hook", err, slog.String("name", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
