package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// RegistryCredentialStore is the store surface the registry credential
// handlers need.
type RegistryCredentialStore interface {
	SaveRegistryCredential(ctx context.Context, c store.RegistryCredential) error
	GetRegistryCredential(ctx context.Context, id string) (store.RegistryCredential, error)
	GetRegistryCredentialByName(ctx context.Context, name string) (store.RegistryCredential, error)
	ListRegistryCredentials(ctx context.Context) ([]store.RegistryCredential, error)
	UpdateRegistryCredential(ctx context.Context, id, name, registryHost, username string) error
	DeleteRegistryCredential(ctx context.Context, id string) error
}

// RegistryCredentialSecretsSetter is the surface registry credential
// creation needs from internal/secrets.Manager, the same "set a value,
// never read one back through this interface" shape BackupSecretsSetter
// already establishes: the password is resolved back out server-side
// only by internal/reconcile/application, never through this package.
type RegistryCredentialSecretsSetter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// registryCredentialResource is the wire shape for a stored registry
// credential. Deliberately no password field, request or response: it's
// accepted only through createRegistryCredentialRequest below and never
// echoed back, the same shape backupTargetResource already establishes.
type registryCredentialResource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RegistryHost string `json:"registry_host"`
	Username     string `json:"username"`
	CreatedAt    string `json:"created_at"`
}

func toRegistryCredentialResource(c store.RegistryCredential) registryCredentialResource {
	return registryCredentialResource{
		ID:           c.ID,
		Name:         c.Name,
		RegistryHost: c.RegistryHost,
		Username:     c.Username,
		CreatedAt:    c.CreatedAt,
	}
}

// createRegistryCredentialRequest is handleCreateRegistryCredential's
// request body: registryCredentialResource's fields plus the one
// write-only credential field no response type ever carries.
type createRegistryCredentialRequest struct {
	Name         string `json:"name"`
	RegistryHost string `json:"registry_host"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

func validateCreateRegistryCredentialRequest(req createRegistryCredentialRequest) error {
	if req.Name == "" {
		return errors.New("name is required")
	}
	if req.RegistryHost == "" {
		return errors.New("registry_host is required")
	}
	if req.Username == "" {
		return errors.New("username is required")
	}
	if req.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

// handleListRegistryCredentials handles GET /api/v1/registry-credentials.
func (rt *Router) handleListRegistryCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := rt.registryCredentials.ListRegistryCredentials(r.Context())
	if err != nil {
		rt.logger.Error("api: list registry credentials failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]registryCredentialResource, 0, len(creds))
	for _, c := range creds {
		out = append(out, toRegistryCredentialResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetRegistryCredential handles GET
// /api/v1/registry-credentials/{id}.
func (rt *Router) handleGetRegistryCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	c, err := rt.registryCredentials.GetRegistryCredential(r.Context(), id)
	if errors.Is(err, store.ErrRegistryCredentialNotFound) {
		writeError(w, http.StatusNotFound, "registry credential not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get registry credential failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toRegistryCredentialResource(c))
}

// updateRegistryCredentialRequest is handleUpdateRegistryCredential's
// request body: registryCredentialResource's fields plus an optional
// Password, the same "blank keeps the existing secret, set to rotate it"
// shape updateBackupTargetRequest establishes for its own credential
// fields.
type updateRegistryCredentialRequest struct {
	Name         string `json:"name"`
	RegistryHost string `json:"registry_host"`
	Username     string `json:"username"`
	Password     string `json:"password,omitempty"`
}

func validateUpdateRegistryCredentialRequest(req updateRegistryCredentialRequest) error {
	if req.Name == "" {
		return errors.New("name is required")
	}
	if req.RegistryHost == "" {
		return errors.New("registry_host is required")
	}
	if req.Username == "" {
		return errors.New("username is required")
	}
	return nil
}

// handleCreateRegistryCredential handles POST /api/v1/registry-credentials.
// AbilityWriteSensitive, the same tier handleCreateBackupTarget requires:
// this endpoint accepts a live registry password in its request body.
//
// The password is written to internal/secrets before the store row, not
// after, the same ordering and the same reasoning
// handleCreateBackupTarget's own doc comment gives: an orphaned secret is
// harmless, a credential row that looks connected but has no working
// password behind it is the failure this ordering avoids.
func (rt *Router) handleCreateRegistryCredential(w http.ResponseWriter, r *http.Request) {
	if rt.registryCredentialSecrets == nil {
		writeError(w, http.StatusNotImplemented, "registry credentials are not configured on this control plane (no master key set)")
		return
	}

	var req createRegistryCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateCreateRegistryCredentialRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err := rt.registryCredentials.GetRegistryCredentialByName(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "a registry credential with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrRegistryCredentialNotFound) {
		rt.logger.Error("api: create registry credential: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := randomRegistryCredentialID()
	if err != nil {
		rt.logger.Error("api: create registry credential: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.registryCredentialSecrets.SetValue(r.Context(), store.RegistryCredentialSecretsKey(id), "password", req.Password); err != nil {
		rt.logger.Error("api: create registry credential: set password failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	cred := store.RegistryCredential{
		ID:           id,
		Name:         req.Name,
		RegistryHost: req.RegistryHost,
		Username:     req.Username,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := rt.registryCredentials.SaveRegistryCredential(r.Context(), cred); err != nil {
		rt.logger.Error("api: create registry credential: save failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toRegistryCredentialResource(cred))
}

// handleUpdateRegistryCredential handles PUT
// /api/v1/registry-credentials/{id}: AbilityWriteSensitive, the same tier
// as create and delete. A full replace of name/registry_host/username;
// the password rotates only when the request includes one, the same
// optional-credential-rotation shape handleUpdateBackupTarget establishes.
func (rt *Router) handleUpdateRegistryCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.registryCredentials.GetRegistryCredential(r.Context(), id); errors.Is(err, store.ErrRegistryCredentialNotFound) {
		writeError(w, http.StatusNotFound, "registry credential not found")
		return
	} else if err != nil {
		rt.logger.Error("api: update registry credential: load failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req updateRegistryCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateUpdateRegistryCredentialRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if existing, err := rt.registryCredentials.GetRegistryCredentialByName(r.Context(), req.Name); err == nil && existing.ID != id {
		writeError(w, http.StatusConflict, "a registry credential with this name already exists")
		return
	} else if err != nil && !errors.Is(err, store.ErrRegistryCredentialNotFound) {
		rt.logger.Error("api: update registry credential: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if req.Password != "" {
		if rt.registryCredentialSecrets == nil {
			writeError(w, http.StatusNotImplemented, "registry credentials are not configured on this control plane (no master key set)")
			return
		}
		if err := rt.registryCredentialSecrets.SetValue(r.Context(), store.RegistryCredentialSecretsKey(id), "password", req.Password); err != nil {
			rt.logger.Error("api: update registry credential: set password failed", slog.String("error", err.Error()), slog.String("id", id))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.registryCredentials.UpdateRegistryCredential(r.Context(), id, req.Name, req.RegistryHost, req.Username); err != nil {
		if errors.Is(err, store.ErrRegistryCredentialNotFound) {
			writeError(w, http.StatusNotFound, "registry credential not found")
			return
		}
		rt.logger.Error("api: update registry credential failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := rt.registryCredentials.GetRegistryCredential(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update registry credential: reload after update failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toRegistryCredentialResource(updated))
}

// handleDeleteRegistryCredential handles DELETE /api/v1/registry-credentials/{id}.
// AbilityWriteSensitive, same as create.
//
// Known gap, left honest rather than silently incomplete, the same one
// handleDeleteBackupTarget's own doc comment already flags for that
// resource: internal/secrets.Manager has no delete/revoke operation
// today, so the password this credential's SetValue call wrote remains
// in the secrets store, unreferenced but not actually erased at rest.
func (rt *Router) handleDeleteRegistryCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := rt.registryCredentials.DeleteRegistryCredential(r.Context(), id)
	if errors.Is(err, store.ErrRegistryCredentialNotFound) {
		writeError(w, http.StatusNotFound, "registry credential not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete registry credential failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// randomRegistryCredentialID mirrors randomBackupTargetID's exact shape.
func randomRegistryCredentialID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate registry credential id: %w", err)
	}
	return "regcred_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
