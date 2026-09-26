package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/models"
)

type modelKeyResource struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	Status      string     `json:"status"`
	RPM         int        `json:"rpm"`
	TPM         int        `json:"tpm"`
	TPD         int        `json:"tpd"`
	MaxParallel int        `json:"max_parallel"`
	AllowPaths  []string   `json:"allow_paths"`
	AllowModels []string   `json:"allow_models"`
	ReplacedBy  string     `json:"replaced_by,omitempty"`
	InFlight    int        `json:"in_flight"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
}

type createdModelKeyResource struct {
	modelKeyResource
	APIKey string `json:"api_key"`
}

type createModelKeyRequest struct {
	Name        string     `json:"name"`
	ExpiresAt   *time.Time `json:"expires_at"`
	RPM         int        `json:"rpm"`
	TPM         int        `json:"tpm"`
	TPD         int        `json:"tpd"`
	MaxParallel int        `json:"max_parallel"`
	AllowPaths  []string   `json:"allow_paths"`
	AllowModels []string   `json:"allow_models"`
}

type rotateModelKeyRequest struct {
	GraceSeconds *int `json:"grace_seconds"`
}

func toModelKeyResource(k models.KeyView) modelKeyResource {
	return modelKeyResource{
		ID: k.ID, Name: k.Name, KeyPrefix: k.KeyPrefix, Status: string(k.Status), RPM: k.RPM, TPM: k.TPM, TPD: k.TPD, CreatedBy: k.CreatedBy, MaxParallel: k.MaxParallel,
		AllowPaths: nonNil(k.AllowPaths), AllowModels: nonNil(k.AllowModels), ReplacedBy: k.ReplacedBy, InFlight: k.InFlight,
		CreatedAt: k.CreatedAt, ExpiresAt: k.ExpiresAt, RevokedAt: k.RevokedAt, LastUsedAt: k.LastUsedAt,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (rt *Router) writeModelKeyError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, models.ErrKeyNotFound):
		writeError(w, http.StatusNotFound, "key not found")
	case errors.Is(err, models.ErrKeyExists):
		writeError(w, http.StatusConflict, "a key with that name already exists")
	default:
		rt.writeModelError(w, op, err)
	}
}

// handleListModelKeys handles GET /api/v1/models/{name}/keys. Key
// material is never returned, only the prefix.
func (rt *Router) handleListModelKeys(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	keys, err := rt.models.ListKeys(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeModelKeyError(w, "list model keys", err)
		return
	}
	out := make([]modelKeyResource, len(keys))
	for i, k := range keys {
		out[i] = toModelKeyResource(k)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateModelKey handles POST /api/v1/models/{name}/keys,
// returning the plaintext key once.
func (rt *Router) handleCreateModelKey(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	var req createModelKeyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	created, err := rt.models.CreateKey(r.Context(), r.PathValue("name"), models.CreateKeyInput{
		Name: req.Name, CreatedBy: rt.eventActor(r), ExpiresAt: req.ExpiresAt,
		Limits: models.KeyLimits{RPM: req.RPM, TPM: req.TPM, TPD: req.TPD, MaxParallel: req.MaxParallel, AllowPaths: req.AllowPaths, AllowModels: req.AllowModels},
	})
	if err != nil {
		rt.writeModelKeyError(w, "create model key", err)
		return
	}
	writeJSON(w, http.StatusCreated, createdModelKeyResource{modelKeyResource: toModelKeyResource(created.KeyView), APIKey: created.Plaintext})
}

// handleRevokeModelKey handles DELETE /api/v1/models/{name}/keys/{id}.
func (rt *Router) handleRevokeModelKey(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	if err := rt.models.RevokeKey(r.Context(), r.PathValue("name"), r.PathValue("id")); err != nil {
		rt.writeModelKeyError(w, "revoke model key", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRotateModelKey handles POST /api/v1/models/{name}/keys/{id}/rotate.
// The body is optional: {"grace_seconds": n} sets how long the old key keeps
// working, 0 for none.
func (rt *Router) handleRotateModelKey(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	var req rotateModelKeyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var grace *time.Duration
	if req.GraceSeconds != nil {
		if *req.GraceSeconds < 0 {
			writeError(w, http.StatusBadRequest, "grace_seconds must not be negative")
			return
		}
		g := time.Duration(*req.GraceSeconds) * time.Second
		grace = &g
	}
	created, err := rt.models.RotateKeyByID(r.Context(), r.PathValue("name"), r.PathValue("id"), rt.eventActor(r), grace)
	if err != nil {
		rt.writeModelKeyError(w, "rotate model key", err)
		return
	}
	writeJSON(w, http.StatusOK, createdModelKeyResource{modelKeyResource: toModelKeyResource(created.KeyView), APIKey: created.Plaintext})
}

// handleModelUsage handles GET /api/v1/models/{name}/usage?since=24h.
func (rt *Router) handleModelUsage(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	window := 24 * time.Hour
	if v := strings.TrimSpace(r.URL.Query().Get("since")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be a duration such as 24h or 168h")
			return
		}
		window = d
	}
	rep, err := rt.models.Usage(r.Context(), r.PathValue("name"), window)
	if err != nil {
		rt.writeModelKeyError(w, "read model usage", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
