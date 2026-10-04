package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Default ports NetworkShareDriverOpts' two protocols listen on, used
// only by handleTestNetworkShare's reachability dial; an operator's own
// mount_options (e.g. a non-standard NFS port) never changes which port
// this check probes, since Docker's own NFS/CIFS mount helpers resolve
// the real port themselves at mount time.
const (
	defaultNFSPort  = 2049
	defaultCIFSPort = 445
)

// networkShareTestTimeout bounds handleTestNetworkShare's synchronous
// dial, the same shape registryCredentialTestTimeout bounds its own
// synchronous auth check.
const networkShareTestTimeout = 10 * time.Second

// NetworkShareStore is the store surface the network share handlers need.
type NetworkShareStore interface {
	SaveNetworkShare(ctx context.Context, s store.NetworkShare) error
	GetNetworkShare(ctx context.Context, id string) (store.NetworkShare, error)
	GetNetworkShareByName(ctx context.Context, name string) (store.NetworkShare, error)
	ListNetworkShares(ctx context.Context) ([]store.NetworkShare, error)
	UpdateNetworkShare(ctx context.Context, id, name, protocol, host, remotePath, mountOptions, username string) error
	DeleteNetworkShare(ctx context.Context, id string) error
}

// NetworkShareSecretsSetter is the surface network share create/update
// needs from internal/secrets.Manager, the same two-method shape
// RegistryCredentialSecretsSetter establishes: SetValue writes a CIFS
// share's password, never read back by any handler in this file.
type NetworkShareSecretsSetter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// networkShareResource is the wire shape for a stored network share.
// Deliberately no password field: accepted only through
// createNetworkShareRequest/updateNetworkShareRequest below and never
// echoed back, the same shape registryCredentialResource establishes.
type networkShareResource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	RemotePath   string `json:"remote_path"`
	MountOptions string `json:"mount_options,omitempty"`
	Username     string `json:"username,omitempty"`
	CreatedAt    string `json:"created_at"`
}

func toNetworkShareResource(s store.NetworkShare) networkShareResource {
	return networkShareResource{
		ID:           s.ID,
		Name:         s.Name,
		Protocol:     s.Protocol,
		Host:         s.Host,
		RemotePath:   s.RemotePath,
		MountOptions: s.MountOptions,
		Username:     s.Username,
		CreatedAt:    s.CreatedAt,
	}
}

// createNetworkShareRequest is handleCreateNetworkShare's request body:
// networkShareResource's fields plus the write-only Password field CIFS
// shares use.
type createNetworkShareRequest struct {
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	RemotePath   string `json:"remote_path"`
	MountOptions string `json:"mount_options,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
}

// validateNetworkShareFields checks the fields create and update share.
// A CIFS share needs a username and password to authenticate; an NFS
// export authenticates by source IP/export rules on the server side, so
// neither is required there.
func validateNetworkShareFields(name, protocol, host, remotePath, username, password string, requirePassword bool) error {
	if name == "" {
		return errors.New("name is required")
	}
	if protocol != store.NetworkShareProtocolNFS && protocol != store.NetworkShareProtocolCIFS {
		return fmt.Errorf("protocol must be %q or %q", store.NetworkShareProtocolNFS, store.NetworkShareProtocolCIFS)
	}
	if host == "" {
		return errors.New("host is required")
	}
	if remotePath == "" {
		return errors.New("remote_path is required")
	}
	if protocol == store.NetworkShareProtocolCIFS {
		if username == "" {
			return errors.New("username is required for a cifs share")
		}
		if requirePassword && password == "" {
			return errors.New("password is required for a cifs share")
		}
	}
	return nil
}

// handleListNetworkShares handles GET /api/v1/network-shares.
func (rt *Router) handleListNetworkShares(w http.ResponseWriter, r *http.Request) {
	shares, err := rt.networkShares.ListNetworkShares(r.Context())
	if err != nil {
		rt.logger.Error("api: list network shares failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]networkShareResource, 0, len(shares))
	for _, s := range shares {
		out = append(out, toNetworkShareResource(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetNetworkShare handles GET /api/v1/network-shares/{id}.
func (rt *Router) handleGetNetworkShare(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s, err := rt.networkShares.GetNetworkShare(r.Context(), id)
	if errors.Is(err, store.ErrNetworkShareNotFound) {
		writeError(w, http.StatusNotFound, "network share not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get network share failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toNetworkShareResource(s))
}

// handleCreateNetworkShare handles POST /api/v1/network-shares.
// AbilityWriteSensitive, the same tier handleCreateRegistryCredential
// requires. The password is written to internal/secrets before the
// store row, not after: an orphaned secret is harmless, a share row with
// no working password behind it is the failure this ordering avoids. An
// NFS share writes no secret at all.
func (rt *Router) handleCreateNetworkShare(w http.ResponseWriter, r *http.Request) {
	var req createNetworkShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateNetworkShareFields(req.Name, req.Protocol, req.Host, req.RemotePath, req.Username, req.Password, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Protocol == store.NetworkShareProtocolCIFS && rt.networkShareSecrets == nil {
		writeError(w, http.StatusNotImplemented, "network shares with credentials are not configured on this control plane (no master key set)")
		return
	}

	_, err := rt.networkShares.GetNetworkShareByName(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "a network share with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrNetworkShareNotFound) {
		rt.logger.Error("api: create network share: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := randomNetworkShareID()
	if err != nil {
		rt.logger.Error("api: create network share: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if req.Protocol == store.NetworkShareProtocolCIFS {
		if err := rt.networkShareSecrets.SetValue(r.Context(), store.NetworkShareSecretsKey(id), "password", req.Password); err != nil {
			rt.logger.Error("api: create network share: set password failed", slog.String("error", err.Error()), slog.String("id", id))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	share := store.NetworkShare{
		ID:           id,
		Name:         req.Name,
		Protocol:     req.Protocol,
		Host:         req.Host,
		RemotePath:   req.RemotePath,
		MountOptions: req.MountOptions,
		Username:     req.Username,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := rt.networkShares.SaveNetworkShare(r.Context(), share); err != nil {
		rt.logger.Error("api: create network share: save failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toNetworkShareResource(share))
}

// updateNetworkShareRequest is handleUpdateNetworkShare's request body:
// networkShareResource's fields plus an optional Password, the same
// "blank keeps the existing secret, set to rotate it" shape
// updateRegistryCredentialRequest establishes.
type updateNetworkShareRequest struct {
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	RemotePath   string `json:"remote_path"`
	MountOptions string `json:"mount_options,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
}

// handleUpdateNetworkShare handles PUT /api/v1/network-shares/{id}:
// AbilityWriteSensitive, same tier as create. A full replace of
// name/protocol/host/remote_path/mount_options/username; a CIFS
// password rotates only when the request includes one.
func (rt *Router) handleUpdateNetworkShare(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.networkShares.GetNetworkShare(r.Context(), id); errors.Is(err, store.ErrNetworkShareNotFound) {
		writeError(w, http.StatusNotFound, "network share not found")
		return
	} else if err != nil {
		rt.logger.Error("api: update network share: load failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req updateNetworkShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateNetworkShareFields(req.Name, req.Protocol, req.Host, req.RemotePath, req.Username, req.Password, false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if existing, err := rt.networkShares.GetNetworkShareByName(r.Context(), req.Name); err == nil && existing.ID != id {
		writeError(w, http.StatusConflict, "a network share with this name already exists")
		return
	} else if err != nil && !errors.Is(err, store.ErrNetworkShareNotFound) {
		rt.logger.Error("api: update network share: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if req.Password != "" {
		if rt.networkShareSecrets == nil {
			writeError(w, http.StatusNotImplemented, "network shares with credentials are not configured on this control plane (no master key set)")
			return
		}
		if err := rt.networkShareSecrets.SetValue(r.Context(), store.NetworkShareSecretsKey(id), "password", req.Password); err != nil {
			rt.logger.Error("api: update network share: set password failed", slog.String("error", err.Error()), slog.String("id", id))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.networkShares.UpdateNetworkShare(r.Context(), id, req.Name, req.Protocol, req.Host, req.RemotePath, req.MountOptions, req.Username); err != nil {
		if errors.Is(err, store.ErrNetworkShareNotFound) {
			writeError(w, http.StatusNotFound, "network share not found")
			return
		}
		rt.logger.Error("api: update network share failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := rt.networkShares.GetNetworkShare(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update network share: reload after update failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toNetworkShareResource(updated))
}

// handleDeleteNetworkShare handles DELETE /api/v1/network-shares/{id}.
// AbilityWriteSensitive, same as create.
//
// Known gap, left honest rather than silently incomplete, the same one
// handleDeleteRegistryCredential's own doc comment flags for that
// resource: internal/secrets.Manager has no delete/revoke operation
// today, so a CIFS share's password remains in the secrets store,
// unreferenced but not actually erased at rest.
func (rt *Router) handleDeleteNetworkShare(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := rt.networkShares.DeleteNetworkShare(r.Context(), id)
	if errors.Is(err, store.ErrNetworkShareNotFound) {
		writeError(w, http.StatusNotFound, "network share not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete network share failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleTestNetworkShare handles POST /api/v1/network-shares/{id}/test:
// dials the share's host on its protocol's standard port to confirm it
// is reachable before anyone attaches it to an app, the same "verify by
// actually reaching the real endpoint, on demand" shape
// handleTestRegistryCredential establishes. This is a reachability
// check, not an authentication check: it never resolves a CIFS
// password, since a closed port makes any later mount fail regardless
// of whether the credential is correct.
func (rt *Router) handleTestNetworkShare(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	share, err := rt.networkShares.GetNetworkShare(r.Context(), id)
	if errors.Is(err, store.ErrNetworkShareNotFound) {
		writeError(w, http.StatusNotFound, "network share not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: test network share: load failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	port := defaultNFSPort
	if share.Protocol == store.NetworkShareProtocolCIFS {
		port = defaultCIFSPort
	}
	addr := net.JoinHostPort(share.Host, strconv.Itoa(port))

	ctx, cancel := context.WithTimeout(r.Context(), networkShareTestTimeout)
	defer cancel()
	conn, err := rt.doctorDialContextOrDefault()(ctx, "tcp", addr)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("could not reach %s: %s", addr, err))
		return
	}
	_ = conn.Close()
	w.WriteHeader(http.StatusNoContent)
}

// randomNetworkShareID mirrors randomRegistryCredentialID's exact shape.
func randomNetworkShareID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate network share id: %w", err)
	}
	return "ns_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
