package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Env seeds for the bootstrap script limits; a runtime setting overrides them.
const (
	envBootstrapScriptMaxBytes = "APP_BOOTSTRAP_SCRIPT_MAX_BYTES"
	envBootstrapScriptMaxCount = "APP_BOOTSTRAP_SCRIPT_MAX_COUNT"

	defaultBootstrapScriptMaxBytes int64 = 64 << 10
	defaultBootstrapScriptMaxCount int64 = 50
	maxBootstrapScriptNameLen            = 64
)

// BootstrapScriptStore is the store surface for saved bootstrap scripts.
// *store.DB satisfies it; a store without it leaves the routes at 501.
type BootstrapScriptStore interface {
	CreateBootstrapScript(ctx context.Context, s store.BootstrapScript) error
	GetBootstrapScript(ctx context.Context, id string) (store.BootstrapScript, error)
	ListBootstrapScripts(ctx context.Context) ([]store.BootstrapScript, error)
	CountBootstrapScripts(ctx context.Context) (int64, error)
	UpdateBootstrapScript(ctx context.Context, id, name, body string, now time.Time) error
	DeleteBootstrapScript(ctx context.Context, id string) error
	GetBootstrapScriptLimits(ctx context.Context) (store.BootstrapScriptLimits, error)
	SetBootstrapScriptLimits(ctx context.Context, l store.BootstrapScriptLimits) error
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
	// Source is stored when either value is a runtime override, else env or default.
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

func bootstrapEnvInt(key string, def int64) (int64, bool) {
	if v, err := strconv.ParseInt(os.Getenv(key), 10, 64); err == nil && v > 0 {
		return v, true
	}
	return def, false
}

func (rt *Router) bootstrapScriptLimits(ctx context.Context) (bootstrapScriptLimitsResource, error) {
	out := bootstrapScriptLimitsResource{Source: "default"}
	var envB, envC bool
	out.MaxBytes, envB = bootstrapEnvInt(envBootstrapScriptMaxBytes, defaultBootstrapScriptMaxBytes)
	out.MaxCount, envC = bootstrapEnvInt(envBootstrapScriptMaxCount, defaultBootstrapScriptMaxCount)
	if envB || envC {
		out.Source = "env"
	}
	stored, err := rt.bootstrapScripts.GetBootstrapScriptLimits(ctx)
	if err != nil {
		return out, fmt.Errorf("api: bootstrap script limits: %w", err)
	}
	if stored.MaxBytes != nil {
		out.MaxBytes, out.Source = *stored.MaxBytes, "stored"
	}
	if stored.MaxCount != nil {
		out.MaxCount, out.Source = *stored.MaxCount, "stored"
	}
	return out, nil
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
	limits, err := rt.bootstrapScriptLimits(r.Context())
	if err != nil {
		rt.internalError(w, "api: create bootstrap script: read limits failed", err)
		return
	}
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
	limits, err := rt.bootstrapScriptLimits(r.Context())
	if err != nil {
		rt.internalError(w, "api: update bootstrap script: read limits failed", err)
		return
	}
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
	res, err := rt.bootstrapScriptLimits(r.Context())
	if err != nil {
		rt.internalError(w, "api: get bootstrap script limits failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handlePutBootstrapScriptLimits handles PUT /api/v1/bootstrap-scripts/limits; root only.
func (rt *Router) handlePutBootstrapScriptLimits(w http.ResponseWriter, r *http.Request) {
	if !rt.bootstrapScriptsReady(w) {
		return
	}
	var req bootstrapScriptLimitsUpdate
	if !decodeReq(w, r, &req) {
		return
	}
	for _, v := range []*int64{req.MaxBytes, req.MaxCount} {
		if v != nil && *v <= 0 {
			writeError(w, http.StatusBadRequest, "limits must be positive, or null to inherit the env default")
			return
		}
	}
	if err := rt.bootstrapScripts.SetBootstrapScriptLimits(r.Context(), store.BootstrapScriptLimits{MaxBytes: req.MaxBytes, MaxCount: req.MaxCount}); err != nil {
		rt.internalError(w, "api: set bootstrap script limits failed", err)
		return
	}
	res, err := rt.bootstrapScriptLimits(r.Context())
	if err != nil {
		rt.internalError(w, "api: set bootstrap script limits: reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// registerBootstrapScriptRoutes registers the saved bootstrap script routes. The literal /limits
// path wins over {id} in net/http's mux.
func (rt *Router) registerBootstrapScriptRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/bootstrap-scripts", rt.requireAbility(AbilityRead, rt.handleListBootstrapScripts))
	mux.HandleFunc("POST /api/v1/bootstrap-scripts", rt.requireAbility(AbilityWrite, rt.handleCreateBootstrapScript))
	mux.HandleFunc("GET /api/v1/bootstrap-scripts/limits", rt.requireAbility(AbilityRead, rt.handleGetBootstrapScriptLimits))
	mux.HandleFunc("PUT /api/v1/bootstrap-scripts/limits", rt.requireAbility(AbilityRoot, rt.handlePutBootstrapScriptLimits))
	mux.HandleFunc("GET /api/v1/bootstrap-scripts/{id}", rt.requireAbility(AbilityRead, rt.handleGetBootstrapScript))
	mux.HandleFunc("PUT /api/v1/bootstrap-scripts/{id}", rt.requireAbility(AbilityWrite, rt.handleUpdateBootstrapScript))
	mux.HandleFunc("DELETE /api/v1/bootstrap-scripts/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteBootstrapScript))
}
