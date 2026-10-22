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
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/pkgregistry"
	"github.com/GLINCKER/levelrail/internal/registryvendor"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

const packageRegistryTestTimeout = 10 * time.Second

// PackageRegistryStore is the store surface the package registry and
// registry credential usage handlers need.
type PackageRegistryStore interface {
	SavePackageRegistry(ctx context.Context, p store.PackageRegistry) error
	GetPackageRegistry(ctx context.Context, id string) (store.PackageRegistry, error)
	ListPackageRegistries(ctx context.Context) ([]store.PackageRegistry, error)
	DeletePackageRegistry(ctx context.Context, id string) error
	ListServicesUsingRegistryCredential(ctx context.Context, credentialID string) ([]string, error)
}

// packageRegistryResource is the wire shape. No secret field exists.
type packageRegistryResource struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Ecosystem      string    `json:"ecosystem"`
	URL            string    `json:"url"`
	Username       string    `json:"username,omitempty"`
	ModulePatterns string    `json:"module_patterns,omitempty"`
	ScopeKind      string    `json:"scope_kind"`
	ScopeRef       string    `json:"scope_ref,omitempty"`
	SecretID       string    `json:"build_secret_id"`
	CreatedBy      string    `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func toPackageRegistryResource(p store.PackageRegistry) packageRegistryResource {
	return packageRegistryResource{
		ID: p.ID, Name: p.Name, Ecosystem: p.Ecosystem, URL: p.URL, Username: p.Username,
		ModulePatterns: p.ModulePatterns, ScopeKind: p.ScopeKind, ScopeRef: p.ScopeRef,
		SecretID: pkgregistry.SecretIDs[p.Ecosystem], CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt,
	}
}

type createPackageRegistryRequest struct {
	Name           string `json:"name"`
	Ecosystem      string `json:"ecosystem"`
	URL            string `json:"url"`
	Username       string `json:"username"`
	Secret         string `json:"secret"`
	ModulePatterns string `json:"module_patterns"`
	ScopeKind      string `json:"scope_kind"`
	ScopeRef       string `json:"scope_ref"`
}

func validatePackageRegistryRequest(req *createPackageRegistryRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return errors.New("name is required")
	}
	if !pkgregistry.IsEcosystem(req.Ecosystem) {
		return fmt.Errorf("ecosystem must be one of %s", strings.Join(pkgregistry.Ecosystems, ", "))
	}
	if err := pkgregistry.ValidateURL(req.URL); err != nil {
		return err
	}
	if req.Secret == "" {
		return errors.New("secret is required")
	}
	if req.ScopeKind == "" {
		req.ScopeKind = pkgregistry.ScopeInstance
	}
	switch req.ScopeKind {
	case pkgregistry.ScopeInstance:
		req.ScopeRef = ""
	case pkgregistry.ScopeProject, pkgregistry.ScopeApp:
		if strings.TrimSpace(req.ScopeRef) == "" {
			return errors.New("scope_ref is required for a project or app scope")
		}
	default:
		return errors.New("scope_kind must be instance, project or app")
	}
	return nil
}

// handleListPackageRegistries handles GET /api/v1/package-registries.
func (rt *Router) handleListPackageRegistries(w http.ResponseWriter, r *http.Request) {
	list, err := rt.packageRegistries.ListPackageRegistries(r.Context())
	if err != nil {
		rt.internalError(w, "api: list package registries failed", err)
		return
	}
	out := make([]packageRegistryResource, 0, len(list))
	for _, p := range list {
		out = append(out, toPackageRegistryResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreatePackageRegistry handles POST /api/v1/package-registries.
// The secret is written before the row so a row never exists without it.
func (rt *Router) handleCreatePackageRegistry(w http.ResponseWriter, r *http.Request) {
	if rt.registryCredentialSecrets == nil {
		writeError(w, http.StatusNotImplemented, "package registries are not configured on this control plane (no master key set)")
		return
	}
	var req createPackageRegistryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validatePackageRegistryRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id := "pkgreg_" + base64.RawURLEncoding.EncodeToString(buf)
	if err := rt.registryCredentialSecrets.SetValue(r.Context(), store.PackageRegistrySecretsKey(id), store.PackageRegistrySecretEnvKey, req.Secret); err != nil {
		rt.internalError(w, "api: save package registry secret failed", err, slog.String("id", id))
		return
	}
	p := store.PackageRegistry{
		ID: id, Name: req.Name, Ecosystem: req.Ecosystem, URL: req.URL, Username: req.Username,
		ModulePatterns: req.ModulePatterns, ScopeKind: req.ScopeKind, ScopeRef: strings.TrimSpace(req.ScopeRef),
		CreatedBy: rt.nodeProviderActor(r), CreatedAt: time.Now().UTC(),
	}
	if err := rt.packageRegistries.SavePackageRegistry(r.Context(), p); err != nil {
		rt.logger.Error("api: save package registry failed", slog.String("id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusConflict, "could not save: a package registry with this name may already exist")
		return
	}
	writeJSON(w, http.StatusCreated, toPackageRegistryResource(p))
}

// handleDeletePackageRegistry handles DELETE /api/v1/package-registries/{id}.
func (rt *Router) handleDeletePackageRegistry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := rt.packageRegistries.DeletePackageRegistry(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrPackageRegistryNotFound) {
			writeError(w, http.StatusNotFound, "package registry not found")
			return
		}
		rt.internalError(w, "api: delete package registry failed", err, slog.String("id", id))
		return
	}
	if deleter, ok := rt.registryCredentialSecrets.(interface {
		DeleteAll(ctx context.Context, serviceName string) error
	}); ok {
		if err := deleter.DeleteAll(r.Context(), store.PackageRegistrySecretsKey(id)); err != nil {
			rt.logger.Warn("api: delete package registry secret failed", slog.String("id", id), slog.String("error", err.Error()))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestPackageRegistry handles POST /api/v1/package-registries/{id}/test:
// one authenticated GET against the registry URL, reporting only the
// outcome class, never the credential.
func (rt *Router) handleTestPackageRegistry(w http.ResponseWriter, r *http.Request) {
	if rt.registryCredentialSecrets == nil {
		writeError(w, http.StatusNotImplemented, "package registries are not configured on this control plane (no master key set)")
		return
	}
	id := r.PathValue("id")
	p, err := rt.packageRegistries.GetPackageRegistry(r.Context(), id)
	if errors.Is(err, store.ErrPackageRegistryNotFound) {
		writeError(w, http.StatusNotFound, "package registry not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: test package registry: load failed", err, slog.String("id", id))
		return
	}
	secret, err := rt.registryCredentialSecrets.Resolve(r.Context(), store.PackageRegistrySecretsKey(id), store.PackageRegistrySecretEnvKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	ok, msg := probePackageRegistry(r.Context(), p, secret)
	writeJSON(w, http.StatusOK, nodeProviderTestResult{OK: ok, Message: msg, TestedAt: time.Now().UTC()})
}

func probePackageRegistry(ctx context.Context, p store.PackageRegistry, secret string) (ok bool, message string) {
	ctx, cancel := context.WithTimeout(ctx, packageRegistryTestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return false, "The registry URL is not valid."
	}
	if p.Username != "" {
		req.SetBasicAuth(p.Username, secret)
	} else {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := netguard.NewClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req) //nolint:gosec // operator supplied URL behind write:sensitive; netguard refuses internal addresses and redirects are not followed so the credential never leaves the host
	if err != nil {
		return false, "Could not reach the registry from the control plane."
	}
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return false, "The registry rejected the credential (" + http.StatusText(resp.StatusCode) + "). Check the username and that the token has read access."
	case resp.StatusCode >= 500:
		return false, "The registry answered with a server error (" + http.StatusText(resp.StatusCode) + ")."
	default:
		return true, "Reachable: the registry accepted the credential."
	}
}

// registryVendorsResponse is GET /api/v1/registry-vendors.
func (rt *Router) handleListRegistryVendors(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, registryvendor.Vendors())
}

type registryCredentialUsageResource struct {
	Services []string `json:"services"`
}

// handleRegistryCredentialUsage handles GET
// /api/v1/registry-credentials/{id}/usage: the services pulling with it.
func (rt *Router) handleRegistryCredentialUsage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	names, err := rt.packageRegistries.ListServicesUsingRegistryCredential(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: registry credential usage failed", err, slog.String("id", id))
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, registryCredentialUsageResource{Services: names})
}
