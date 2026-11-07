package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxBootstrapScriptNameLen = 64

// BootstrapScriptStore is the store surface for saved bootstrap scripts.
// *store.DB satisfies it; a store without it leaves the routes at 501.
type BootstrapScriptStore interface {
	CreateBootstrapScript(ctx context.Context, s store.BootstrapScript) error
	GetBootstrapScript(ctx context.Context, id string) (store.BootstrapScript, error)
	ListBootstrapScripts(ctx context.Context) ([]store.BootstrapScript, error)
	CountBootstrapScripts(ctx context.Context) (int64, error)
	UpdateBootstrapScript(ctx context.Context, id, name, body string, now time.Time) error
	DeleteBootstrapScript(ctx context.Context, id string) error
}

func bootstrapScriptStoreOf(s any) BootstrapScriptStore {
	b, _ := s.(BootstrapScriptStore)
	return b
}

// bootstrapScriptResource is the wire shape of one script. The list omits Body and reports its size.
type bootstrapScriptResource struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body,omitempty"`
	SizeBytes int       `json:"size_bytes"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type bootstrapScriptRequest struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

// bootstrapScriptLimitsResource is the wire shape of GET and PUT /api/v1/bootstrap-scripts/limits.
type bootstrapScriptLimitsResource struct {
	MaxBytes int64 `json:"max_bytes"`
	MaxCount int64 `json:"max_count"`
	// Source is saved when either value is a runtime override, else env or default.
	Source string `json:"source"`
}

// bootstrapScriptLimitsUpdate nulls clear an override so the env seed applies again.
type bootstrapScriptLimitsUpdate struct {
	MaxBytes *int64 `json:"max_bytes"`
	MaxCount *int64 `json:"max_count"`
}

func bootstrapScriptView(s store.BootstrapScript, withBody bool) bootstrapScriptResource {
	res := bootstrapScriptResource{ID: s.ID, Name: s.Name, SizeBytes: len(s.Body), CreatedBy: s.CreatedBy, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	if withBody {
		res.Body = s.Body
	}
	return res
}

// bootstrapLimit reads one registered limit; a non-positive value falls back to the default so a bad env seed never locks uploads.
func (rt *Router) bootstrapLimit(ctx context.Context, key string, def int64) (int64, string) {
	n := int64(rt.runtimeSettings.Int(ctx, key))
	src := rt.runtimeSettings.Source(ctx, key)
	if n <= 0 {
		n = def
		src = runtimesettings.SourceDefault
	}
	return n, src
}

func (rt *Router) bootstrapScriptLimits(ctx context.Context) bootstrapScriptLimitsResource {
	out := bootstrapScriptLimitsResource{Source: runtimesettings.SourceDefault}
	var bytesSrc, countSrc string
	out.MaxBytes, bytesSrc = rt.bootstrapLimit(ctx, runtimesettings.KeyBootstrapScriptMaxBytes, runtimesettings.DefaultBootstrapScriptMaxBytes)
	out.MaxCount, countSrc = rt.bootstrapLimit(ctx, runtimesettings.KeyBootstrapScriptMaxCount, runtimesettings.DefaultBootstrapScriptMaxCount)
	for _, src := range []string{bytesSrc, countSrc} {
		if src == runtimesettings.SourceSaved || (src == runtimesettings.SourceEnv && out.Source != runtimesettings.SourceSaved) {
			out.Source = src
		}
	}
	return out
}

func validateBootstrapScript(req bootstrapScriptRequest, maxBytes int64) string {
	name := strings.TrimSpace(req.Name)
	switch {
	case name == "" || len(name) > maxBootstrapScriptNameLen || strings.ContainsAny(name, "\r\n"):
		return fmt.Sprintf("name is required, at most %d characters and a single line", maxBootstrapScriptNameLen)
	case strings.TrimSpace(req.Body) == "":
		return "body is required"
	case int64(len(req.Body)) > maxBytes:
		return fmt.Sprintf("body exceeds the %d byte limit", maxBytes)
	case !utf8.ValidString(req.Body) || strings.ContainsRune(req.Body, 0):
		return "body must be UTF-8 text without NUL bytes"
	}
	return ""
}

func randomBootstrapScriptID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate bootstrap script id: %w", err)
	}
	return "bscript_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func (rt *Router) bootstrapScriptsReady(w http.ResponseWriter) bool {
	if rt.bootstrapScripts == nil {
		writeError(w, http.StatusNotImplemented, "bootstrap scripts are not configured on this control plane")
		return false
	}
	return true
}

// handleListBootstrapScripts handles GET /api/v1/bootstrap-scripts.
func (rt *Router) handleListBootstrapScripts(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	list, err := rt.bootstrapScripts.ListBootstrapScripts(r.Context())
	if err != nil {
		rt.internalError(w, "api: list bootstrap scripts failed", err)
		return
	}
	out := make([]bootstrapScriptResource, 0, len(list))
	for _, s := range list {
		out = append(out, bootstrapScriptView(s, false))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetBootstrapScript handles GET /api/v1/bootstrap-scripts/{id}.
func (rt *Router) handleGetBootstrapScript(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	s, err := rt.bootstrapScripts.GetBootstrapScript(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrBootstrapScriptNotFound) {
		writeError(w, http.StatusNotFound, "bootstrap script not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get bootstrap script failed", err)
		return
	}
	writeJSON(w, http.StatusOK, bootstrapScriptView(s, true))
}

// handleCreateBootstrapScript handles POST /api/v1/bootstrap-scripts. The body is stored as opaque
// text and only ever runs on a new node's first boot, never on the control plane.
func (rt *Router) handleCreateBootstrapScript(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	limits := rt.bootstrapScriptLimits(r.Context())
	var req bootstrapScriptRequest
	if !decodeReqLimit(w, r, &req, limits.MaxBytes+4096) {
		return
	}
	if msg := validateBootstrapScript(req, limits.MaxBytes); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	n, err := rt.bootstrapScripts.CountBootstrapScripts(r.Context())
	if err != nil {
		rt.internalError(w, "api: create bootstrap script: count failed", err)
		return
	}
	if n >= limits.MaxCount {
		writeError(w, http.StatusConflict, fmt.Sprintf("the limit of %d bootstrap scripts is reached", limits.MaxCount))
		return
	}
	id, err := randomBootstrapScriptID()
	if err != nil {
		rt.internalError(w, "api: create bootstrap script: id failed", err)
		return
	}
	now := time.Now()
	rec := store.BootstrapScript{ID: id, Name: strings.TrimSpace(req.Name), Body: req.Body, CreatedBy: rt.actorLabel(r), CreatedAt: now, UpdatedAt: now}
	if err := rt.bootstrapScripts.CreateBootstrapScript(r.Context(), rec); err != nil {
		if errors.Is(err, store.ErrBootstrapScriptNameTaken) {
			writeError(w, http.StatusConflict, "a bootstrap script with this name already exists")
			return
		}
		rt.internalError(w, "api: create bootstrap script failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, bootstrapScriptView(rec, true))
}

// handleUpdateBootstrapScript handles PUT /api/v1/bootstrap-scripts/{id}: a full replace of name and body.
func (rt *Router) handleUpdateBootstrapScript(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	limits := rt.bootstrapScriptLimits(r.Context())
	var req bootstrapScriptRequest
	if !decodeReqLimit(w, r, &req, limits.MaxBytes+4096) {
		return
	}
	if msg := validateBootstrapScript(req, limits.MaxBytes); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	id := r.PathValue("id")
	if err := rt.bootstrapScripts.UpdateBootstrapScript(r.Context(), id, strings.TrimSpace(req.Name), req.Body, time.Now()); err != nil {
		switch {
		case errors.Is(err, store.ErrBootstrapScriptNotFound):
			writeError(w, http.StatusNotFound, "bootstrap script not found")
		case errors.Is(err, store.ErrBootstrapScriptNameTaken):
			writeError(w, http.StatusConflict, "a bootstrap script with this name already exists")
		default:
			rt.internalError(w, "api: update bootstrap script failed", err)
		}
		return
	}
	s, err := rt.bootstrapScripts.GetBootstrapScript(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: update bootstrap script: reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, bootstrapScriptView(s, true))
}

// handleDeleteBootstrapScript handles DELETE /api/v1/bootstrap-scripts/{id}.
func (rt *Router) handleDeleteBootstrapScript(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	err := rt.bootstrapScripts.DeleteBootstrapScript(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrBootstrapScriptNotFound) {
		writeError(w, http.StatusNotFound, "bootstrap script not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete bootstrap script failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetBootstrapScriptLimits handles GET /api/v1/bootstrap-scripts/limits.
func (rt *Router) handleGetBootstrapScriptLimits(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	writeJSON(w, http.StatusOK, rt.bootstrapScriptLimits(r.Context()))
}

// handlePutBootstrapScriptLimits handles PUT /api/v1/bootstrap-scripts/limits; root only.
func (rt *Router) handlePutBootstrapScriptLimits(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	if rt.runtimeSettings == nil {
		writeError(w, http.StatusNotImplemented, "runtime settings are not configured on this control plane")
		return
	}
	var req bootstrapScriptLimitsUpdate
	if !decodeReq(w, r, &req) {
		return
	}
	actor := rt.actorForRequest(r)
	for key, v := range map[string]*int64{
		runtimesettings.KeyBootstrapScriptMaxBytes: req.MaxBytes,
		runtimesettings.KeyBootstrapScriptMaxCount: req.MaxCount,
	} {
		var err error
		if v == nil {
			_, err = rt.runtimeSettings.Reset(r.Context(), key)
		} else {
			_, err = rt.runtimeSettings.Set(r.Context(), key, json.RawMessage(strconv.FormatInt(*v, 10)), actor)
		}
		var verr *runtimesettings.ValidationError
		if errors.As(err, &verr) {
			writeError(w, http.StatusBadRequest, verr.Message)
			return
		}
		if err != nil {
			rt.internalError(w, "api: set bootstrap script limits failed", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, rt.bootstrapScriptLimits(r.Context()))
}

// registerBootstrapScriptRoutes registers the saved bootstrap script routes. The literal /limits
// path wins over {id} in net/http's mux.
func (rt *Router) registerBootstrapScriptRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/bootstrap-scripts", rt.requireAbility(AbilityRead, rt.handleListBootstrapScripts))
	mux.HandleFunc("POST /api/v1/bootstrap-scripts", rt.requireAbility(AbilityRoot, rt.handleCreateBootstrapScript))
	mux.HandleFunc("GET /api/v1/bootstrap-scripts/limits", rt.requireAbility(AbilityRead, rt.handleGetBootstrapScriptLimits))
	mux.HandleFunc("PUT /api/v1/bootstrap-scripts/limits", rt.requireAbility(AbilityRoot, rt.handlePutBootstrapScriptLimits))
	mux.HandleFunc("GET /api/v1/bootstrap-scripts/{id}", rt.requireAbility(AbilityRoot, rt.handleGetBootstrapScript))
	mux.HandleFunc("PUT /api/v1/bootstrap-scripts/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateBootstrapScript))
	mux.HandleFunc("DELETE /api/v1/bootstrap-scripts/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteBootstrapScript))
}
