package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/registrycatalog"
	"github.com/GLINCKER/levelrail/internal/registrytoken"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// registryCredentialTestTimeout bounds the synchronous auth check
// handleTestRegistryCredential runs, the same shape
// notificationChannelTestTimeout bounds its own synchronous test-send.
const registryCredentialTestTimeout = 10 * time.Second

// RegistryCredentialStore is the store surface the registry credential
// handlers need.
type RegistryCredentialStore interface {
	SaveRegistryCredential(ctx context.Context, c store.RegistryCredential) error
	GetRegistryCredential(ctx context.Context, id string) (store.RegistryCredential, error)
	GetRegistryCredentialByName(ctx context.Context, name string) (store.RegistryCredential, error)
	ListRegistryCredentials(ctx context.Context) ([]store.RegistryCredential, error)
	UpdateRegistryCredential(ctx context.Context, id, name, registryHost, username string, expiresAt *time.Time) error
	DeleteRegistryCredential(ctx context.Context, id string) error
}

// RegistryCredentialSecretsSetter is the surface registry credential
// create/update/test/browse needs from internal/secrets.Manager. Resolve
// is used by handleTestRegistryCredential and the two credential browse
// handlers in registry_catalog.go, to authenticate against the real
// registry on the operator's behalf; it is never used to echo a password
// back in an HTTP response, the same boundary GitSourceSecrets already
// draws for its own Resolve use.
type RegistryCredentialSecretsSetter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// registryCredentialResource is the wire shape for a stored registry
// credential. Deliberately no password field, request or response: it's
// accepted only through createRegistryCredentialRequest below and never
// echoed back, the same shape backupTargetResource already establishes.
type registryCredentialResource struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	RegistryHost string     `json:"registry_host"`
	Username     string     `json:"username"`
	CreatedAt    string     `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	// ExpiryStatus is "healthy", "expiring_soon", or "expired"
	// (alerting.CertExpiryStatus, the same three-state bucketing
	// certificateStatus.Status already uses), computed from ExpiresAt.
	// Omitted entirely when no expiry was set: this platform has no way
	// to infer one from an opaque credential string, so "unknown" is a
	// real, distinct state from "healthy".
	ExpiryStatus string `json:"expiry_status,omitempty"`
	// Preset is "ecr", "artifact_registry" or "acr" for a self-refreshing
	// credential. The remaining fields describe its refresh state; none of
	// them is secret.
	Preset           string     `json:"preset,omitempty"`
	Region           string     `json:"region,omitempty"`
	LastRefreshedAt  *time.Time `json:"last_refreshed_at,omitempty"`
	NextRefreshAt    *time.Time `json:"next_refresh_at,omitempty"`
	LastRefreshError string     `json:"last_refresh_error,omitempty"`
}

// toRegistryCredentialResource maps a stored credential to its wire
// shape. A method on Router, not a plain function, so ExpiryStatus can
// reuse rt.certExpiryWarningWindow: the same "how far ahead to warn"
// threshold GET /api/v1/certificates already applies, rather than a
// second hardcoded/env-configured window for what is the same underlying
// question an operator asks of either resource.
func (rt *Router) toRegistryCredentialResource(c store.RegistryCredential) registryCredentialResource {
	res := registryCredentialResource{
		ID:           c.ID,
		Name:         c.Name,
		RegistryHost: c.RegistryHost,
		Username:     c.Username,
		CreatedAt:    c.CreatedAt,
		ExpiresAt:    c.ExpiresAt,

		Preset:           c.Preset,
		LastRefreshedAt:  c.LastRefreshedAt,
		NextRefreshAt:    c.NextRefreshAt,
		LastRefreshError: c.LastRefreshError,
	}
	if c.Preset != "" {
		if cfg, err := registrytoken.ParseConfig(c.PresetConfig); err == nil {
			res.Region = cfg.Region
		}
	}
	if c.ExpiresAt != nil {
		warningWindow := rt.certExpiryWarningWindow
		if warningWindow <= 0 {
			warningWindow = alerting.DefaultCertExpiryWarningWindow
		}
		res.ExpiryStatus = alerting.CertExpiryStatus(*c.ExpiresAt, time.Now(), warningWindow)
	}
	return res
}

// createRegistryCredentialRequest is handleCreateRegistryCredential's
// request body: registryCredentialResource's fields plus the one
// write-only credential field no response type ever carries.
type createRegistryCredentialRequest struct {
	Name         string     `json:"name"`
	RegistryHost string     `json:"registry_host"`
	Username     string     `json:"username"`
	Password     string     `json:"password"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	// Preset and Region select a self-refreshing cloud login; password and
	// username are then minted, not supplied.
	Preset string `json:"preset,omitempty"`
	Region string `json:"region,omitempty"`
}

func validateCreateRegistryCredentialRequest(req createRegistryCredentialRequest) error {
	if req.Name == "" {
		return errors.New("name is required")
	}
	if req.Preset != "" {
		return nil
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
		rt.internalError(w, "api: list registry credentials failed", err)
		return
	}
	out := make([]registryCredentialResource, 0, len(creds))
	for _, c := range creds {
		out = append(out, rt.toRegistryCredentialResource(c))
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
		rt.internalError(w, "api: get registry credential failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.toRegistryCredentialResource(c))
}

// updateRegistryCredentialRequest is handleUpdateRegistryCredential's
// request body: registryCredentialResource's fields plus an optional
// Password, the same "blank keeps the existing secret, set to rotate it"
// shape updateBackupTargetRequest establishes for its own credential
// fields.
type updateRegistryCredentialRequest struct {
	Name         string     `json:"name"`
	RegistryHost string     `json:"registry_host"`
	Username     string     `json:"username"`
	Password     string     `json:"password,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
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
	if !decodeReq(w, r, &req) {
		return
	}
	if err := validateCreateRegistryCredentialRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Preset == "" {
		if err := rt.checkRegistryHost(r.Context(), req.RegistryHost); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	_, err := rt.registryCredentials.GetRegistryCredentialByName(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "a registry credential with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrRegistryCredentialNotFound) {
		rt.internalError(w, "api: create registry credential: check existing failed", err, slog.String("name", req.Name))
		return
	}
	if req.Preset != "" {
		rt.createPresetRegistryCredential(w, r, req)
		return
	}

	id, err := randomRegistryCredentialID()
	if err != nil {
		rt.internalError(w, "api: create registry credential: generate id failed", err)
		return
	}

	if err := rt.registryCredentialSecrets.SetValue(r.Context(), store.RegistryCredentialSecretsKey(id), "password", req.Password); err != nil {
		rt.internalError(w, "api: create registry credential: set password failed", err, slog.String("id", id))
		return
	}

	cred := store.RegistryCredential{
		ID:           id,
		Name:         req.Name,
		RegistryHost: req.RegistryHost,
		Username:     req.Username,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:    req.ExpiresAt,
	}
	if err := rt.registryCredentials.SaveRegistryCredential(r.Context(), cred); err != nil {
		rt.internalError(w, "api: create registry credential: save failed", err, slog.String("id", id))
		return
	}

	writeJSON(w, http.StatusCreated, rt.toRegistryCredentialResource(cred))
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
		rt.internalError(w, "api: update registry credential: load failed", err, slog.String("id", id))
		return
	}

	var req updateRegistryCredentialRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if err := validateUpdateRegistryCredentialRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.checkRegistryHost(r.Context(), req.RegistryHost); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if existing, err := rt.registryCredentials.GetRegistryCredentialByName(r.Context(), req.Name); err == nil && existing.ID != id {
		writeError(w, http.StatusConflict, "a registry credential with this name already exists")
		return
	} else if err != nil && !errors.Is(err, store.ErrRegistryCredentialNotFound) {
		rt.internalError(w, "api: update registry credential: check existing failed", err, slog.String("name", req.Name))
		return
	}

	if req.Password != "" {
		if rt.registryCredentialSecrets == nil {
			writeError(w, http.StatusNotImplemented, "registry credentials are not configured on this control plane (no master key set)")
			return
		}
		if err := rt.registryCredentialSecrets.SetValue(r.Context(), store.RegistryCredentialSecretsKey(id), "password", req.Password); err != nil {
			rt.internalError(w, "api: update registry credential: set password failed", err, slog.String("id", id))
			return
		}
	}

	if err := rt.registryCredentials.UpdateRegistryCredential(r.Context(), id, req.Name, req.RegistryHost, req.Username, req.ExpiresAt); err != nil {
		if errors.Is(err, store.ErrRegistryCredentialNotFound) {
			writeError(w, http.StatusNotFound, "registry credential not found")
			return
		}
		rt.internalError(w, "api: update registry credential failed", err, slog.String("id", id))
		return
	}

	updated, err := rt.registryCredentials.GetRegistryCredential(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: update registry credential: reload after update failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.toRegistryCredentialResource(updated))
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
		rt.internalError(w, "api: delete registry credential failed", err, slog.String("id", id))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleTestRegistryCredential handles POST
// /api/v1/registry-credentials/{id}/test: authenticates the stored
// username/password against RegistryHost via the same Docker daemon
// ensureImage (internal/docker/client.go) uses for a real image pull,
// without pulling anything. Catches a bad or stale credential before a
// deploy fails mid-pull, the same "verify a stored credential by
// actually using it, once, on demand" shape
// handleTestExistingNotificationChannel already establishes.
func (rt *Router) handleTestRegistryCredential(w http.ResponseWriter, r *http.Request) {
	if rt.registryAuthTester == nil || rt.registryCredentialSecrets == nil {
		writeError(w, http.StatusNotImplemented, "registry credential testing is not configured on this control plane")
		return
	}

	id := r.PathValue("id")
	cred, err := rt.registryCredentials.GetRegistryCredential(r.Context(), id)
	if errors.Is(err, store.ErrRegistryCredentialNotFound) {
		writeError(w, http.StatusNotFound, "registry credential not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: test registry credential: load failed", err, slog.String("id", id))
		return
	}

	password, err := rt.registryCredentialSecrets.Resolve(r.Context(), store.RegistryCredentialSecretsKey(id), "password")
	if err != nil {
		rt.internalError(w, "api: test registry credential: resolve password failed", err, slog.String("id", id))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), registryCredentialTestTimeout)
	defer cancel()
	if err := rt.registryAuthTester.TestRegistryAuth(ctx, cred.RegistryHost, cred.Username, password); err != nil {
		rt.logger.Warn("api: registry credential test failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusBadGateway, registryAuthTestErrorMessage(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// registryAuthTestErrorMessage maps a failed TestRegistryAuth error to a
// coarse category so daemon detail (addresses, resolver output) is never
// returned to the caller; the detail is logged by the handler.
func registryAuthTestErrorMessage(err error) string {
	if cerrdefs.IsUnauthorized(err) {
		return "authentication rejected by registry: check the username and password"
	}
	low := strings.ToLower(err.Error())
	var netErr net.Error
	switch {
	case strings.Contains(low, "x509") || strings.Contains(low, "tls:") || strings.Contains(low, "certificate"):
		return "could not reach registry: TLS error"
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) ||
		strings.Contains(low, "no such host") || strings.Contains(low, "connection refused") || strings.Contains(low, "timeout"):
		return "could not reach registry: unreachable"
	}
	return "could not verify the registry credential"
}

// checkRegistryHost validates a registry address before it is stored or
// used. The platform registry's own host is exempt so it keeps working
// on private networks.
func (rt *Router) checkRegistryHost(ctx context.Context, host string) error {
	exempt := ""
	if rt.registry != nil {
		if settings, err := rt.registry.GetRegistrySettings(ctx); err == nil {
			exempt = settings.Host
		}
	}
	return validateRegistryHost(ctx, host, exempt, registrycatalog.AllowPrivate())
}

// registryHostname extracts the bare hostname from a host[:port] or
// scheme://host[:port] registry address, rejecting anything else.
func registryHostname(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n@?#") {
		return "", errors.New("registry_host must be a plain host or host:port")
	}
	target := raw
	if !strings.Contains(raw, "://") {
		target = "https://" + raw
	}
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("registry_host must be a plain host or host:port")
	}
	return strings.ToLower(u.Hostname()), nil
}

// validateRegistryHost checks the address shape, then refuses internal
// targets unless allowPrivate is set. Link-local and metadata stay
// refused even then. exempt is the built-in registry host, if any.
func validateRegistryHost(ctx context.Context, raw, exempt string, allowPrivate bool) error {
	name, err := registryHostname(raw)
	if err != nil {
		return err
	}
	if exempt != "" {
		if ex, exErr := registryHostname(exempt); exErr == nil && ex == name {
			return nil
		}
	}
	errInternal := errors.New("registry_host points at an internal address; set " + registrycatalog.AllowPrivateEnv + " to allow private registries")
	blocked := func(a netip.Addr) bool {
		if allowPrivate {
			return !netguard.IsPrivateTargetAllowed(a)
		}
		return netguard.IsBlocked(a)
	}
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		if allowPrivate {
			return nil
		}
		return errInternal
	}
	if ip, perr := netip.ParseAddr(name); perr == nil {
		if blocked(ip) {
			return errInternal
		}
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, lerr := net.DefaultResolver.LookupNetIP(lookupCtx, "ip", name)
	if lerr != nil || len(addrs) == 0 {
		return nil
	}
	for _, a := range addrs {
		if !blocked(a) {
			return nil
		}
	}
	return errInternal
}

// randomRegistryCredentialID mirrors randomBackupTargetID's exact shape.
func randomRegistryCredentialID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate registry credential id: %w", err)
	}
	return "regcred_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
