package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/GLINCKER/levelrail/internal/store"
)

const revealBodyLimit = 16 << 10

type revealRequest struct {
	Key    string       `json:"key"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

type revealResponse struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	Type             string `json:"type"`
	HideAfterSeconds int    `json:"hide_after_seconds"`
}

// Variable types the editor shows in its Type column.
const (
	envTypePlain  = "plain"
	envTypeSecret = "secret"
	envTypeVault  = "vault"
)

func decodeRevealBody(w http.ResponseWriter, r *http.Request, into *revealRequest) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, revealBodyLimit)).Decode(into)
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func (rt *Router) loadRevealApp(w http.ResponseWriter, r *http.Request, name string) (*store.DesiredService, bool) {
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: reveal: load app failed", err, slog.String("name", name))
		return nil, false
	}
	return svc, true
}

// resolveRevealValue reads one real value server side: a plain variable
// from desired state, or a secret from envelope storage.
func (rt *Router) resolveRevealValue(w http.ResponseWriter, r *http.Request, svc *store.DesiredService, key string) (string, string, bool) {
	if v, ok := svc.Env[key]; ok {
		return v, envTypePlain, true
	}
	if _, ok := svc.VaultEnv[key]; ok {
		writeError(w, http.StatusBadRequest, "this value is read from Vault at deploy time and is never stored here")
		return "", "", false
	}
	if !slices.Contains(store.SecretEnvNames(svc.SecretEnv), key) {
		writeError(w, http.StatusNotFound, "no variable with this key on this app")
		return "", "", false
	}
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return "", "", false
	}
	v, err := rt.secrets.Resolve(r.Context(), svc.Name, key)
	if err != nil {
		rt.internalError(w, "api: reveal: resolve secret failed", err, slog.String("name", svc.Name), slog.String("key", key))
		return "", "", false
	}
	return v, envTypeSecret, true
}

// handleRevealEnv handles POST /api/v1/apps/{name}/env/reveal: one value,
// after step-up, audited without the value.
func (rt *Router) handleRevealEnv(w http.ResponseWriter, r *http.Request) {
	var req revealRequest
	if !decodeRevealBody(w, r, &req) {
		return
	}
	rt.revealOne(w, r, r.PathValue("name"), req.Key, req.StepUp)
}

// handleRevealSecret handles POST /api/v1/apps/{name}/secrets/{key}/reveal.
func (rt *Router) handleRevealSecret(w http.ResponseWriter, r *http.Request) {
	var req revealRequest
	if !decodeRevealBody(w, r, &req) {
		return
	}
	rt.revealOne(w, r, r.PathValue("name"), r.PathValue("key"), req.StepUp)
}

func (rt *Router) revealOne(w http.ResponseWriter, r *http.Request, name, key string, proof *stepUpProof) {
	noStore(w)
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	actor, ok := rt.authorizeReveal(w, r, name, stepUpActionReveal, proof, revealOptions{})
	if !ok {
		return
	}
	value, kind, ok := rt.resolveRevealValue(w, r, svc, key)
	if !ok {
		return
	}
	rt.saveEnvAudit(r.Context(), r, actor, auditActionEnvRevealed, "apps/"+name+"/env/"+key, http.StatusOK)
	rt.logger.Info("api: env value revealed", slog.String("name", name), slog.String("key", key), slog.String("actor_id", actor.id), slog.String("method", actor.method))
	writeJSON(w, http.StatusOK, revealResponse{Key: key, Value: value, Type: kind, HideAfterSeconds: int(revealHideAfter().Seconds())})
}
