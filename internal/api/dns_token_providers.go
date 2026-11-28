package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/glincker/levelrail/internal/dnsrecords"
	"github.com/glincker/levelrail/internal/dnszones"
	"github.com/glincker/levelrail/internal/store"
)

const dnsTokenNoMasterKey = "dns provider settings are not configured on this control plane (no master key set)" //nolint:gosec // message text, not a credential

// DNSTokenStore is the store surface the /api/v1/settings/dns-providers routes need.
type DNSTokenStore interface {
	GetDNSTokenProviderEnabled(ctx context.Context, provider string) (bool, error)
	SetDNSTokenProviderEnabled(ctx context.Context, provider string, enabled bool) error
}

// DNSTokenSecrets is the same secrets surface Cloud DNS uses.
type DNSTokenSecrets = GoogleCloudDNSSecrets

// dnsTokenResource is the wire shape of the settings routes. The credential never appears in it.
type dnsTokenResource struct {
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
	HasKey   bool   `json:"has_key"`
	// Verification appears only on a PUT that saved a new credential.
	Verification *dnsTokenVerification `json:"verification,omitempty"`
}

type updateDNSTokenRequest struct {
	Enabled bool `json:"enabled"`
	// Credential is the API token (DigitalOcean, Hetzner) or the credential JSON (Azure). Empty keeps the stored one.
	Credential string `json:"credential,omitempty"`
}

type dnsTokenTestResponse struct {
	OK    bool     `json:"ok"`
	Zones []string `json:"zones,omitempty"`
	Error string   `json:"error,omitempty"`
}

func (rt *Router) dnsTokenProviderName(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("provider")
	if !dnszones.IsTokenProvider(name) {
		writeError(w, http.StatusNotFound, "unknown dns provider")
		return "", false
	}
	return name, true
}

func (rt *Router) dnsTokenResource(ctx context.Context, name string, enabled bool) dnsTokenResource {
	res := dnsTokenResource{Provider: name, Enabled: enabled}
	if rt.dnsTokenSecrets != nil {
		has, err := rt.dnsTokenSecrets.Exists(ctx, store.DNSTokenSecretsKey(name), store.DNSTokenEnvKey)
		if err != nil {
			rt.logger.Warn("api: check dns provider credential failed", slog.String("provider", name), slog.String("error", err.Error()))
		}
		res.HasKey = has
	}
	return res
}

// handleGetDNSTokenProvider handles GET /api/v1/settings/dns-providers/{provider}.
func (rt *Router) handleGetDNSTokenProvider(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.dnsTokenProviderName(w, r)
	if !ok {
		return
	}
	enabled, err := rt.dnsTokenStore.GetDNSTokenProviderEnabled(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get dns provider failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.dnsTokenResource(r.Context(), name, enabled))
}

// handleUpdateDNSTokenProvider handles PUT /api/v1/settings/dns-providers/{provider}.
func (rt *Router) handleUpdateDNSTokenProvider(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.dnsTokenProviderName(w, r)
	if !ok {
		return
	}
	if rt.dnsTokenSecrets == nil {
		writeError(w, http.StatusNotImplemented, dnsTokenNoMasterKey)
		return
	}
	var req updateDNSTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	key := store.DNSTokenSecretsKey(name)
	if req.Credential != "" {
		if _, err := rt.newDNSTokenProvider(name, req.Credential); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := rt.dnsTokenSecrets.SetValue(ctx, key, store.DNSTokenEnvKey, req.Credential); err != nil {
			rt.internalError(w, "api: save dns provider credential failed", err)
			return
		}
	} else if req.Enabled {
		has, err := rt.dnsTokenSecrets.Exists(ctx, key, store.DNSTokenEnvKey)
		if err != nil {
			rt.internalError(w, "api: check dns provider credential failed", err)
			return
		}
		if !has {
			writeError(w, http.StatusBadRequest, "a credential is required the first time this dns provider is enabled")
			return
		}
	}
	if err := rt.dnsTokenStore.SetDNSTokenProviderEnabled(ctx, name, req.Enabled); err != nil {
		rt.internalError(w, "api: update dns provider failed", err)
		return
	}
	res := rt.dnsTokenResource(ctx, name, req.Enabled)
	if req.Credential != "" {
		res.Verification = rt.verifyDNSCredential(ctx, name, func() (dnszones.Provider, error) {
			return rt.newDNSTokenProvider(name, req.Credential)
		})
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDisconnectDNSTokenProvider handles DELETE /api/v1/settings/dns-providers/{provider}.
func (rt *Router) handleDisconnectDNSTokenProvider(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.dnsTokenProviderName(w, r)
	if !ok {
		return
	}
	if rt.dnsTokenSecrets == nil {
		writeError(w, http.StatusNotImplemented, dnsTokenNoMasterKey)
		return
	}
	if err := rt.dnsTokenSecrets.DeleteAll(r.Context(), store.DNSTokenSecretsKey(name)); err != nil {
		rt.internalError(w, "api: clear dns provider credential failed", err)
		return
	}
	if err := rt.dnsTokenStore.SetDNSTokenProviderEnabled(r.Context(), name, false); err != nil {
		rt.internalError(w, "api: disconnect dns provider failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.dnsTokenResource(r.Context(), name, false))
}

// handleTestDNSTokenProvider handles POST /api/v1/settings/dns-providers/{provider}/test.
// A provider failure is reported in the body (ok=false), not as an HTTP error.
func (rt *Router) handleTestDNSTokenProvider(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.dnsTokenProviderName(w, r)
	if !ok {
		return
	}
	if rt.dnsTokenSecrets == nil {
		writeError(w, http.StatusNotImplemented, dnsTokenNoMasterKey)
		return
	}
	raw, err := rt.dnsTokenSecrets.Resolve(r.Context(), store.DNSTokenSecretsKey(name), store.DNSTokenEnvKey)
	if err != nil || raw == "" {
		writeJSON(w, http.StatusOK, dnsTokenTestResponse{Error: "no credential is stored yet"})
		return
	}
	p, err := rt.newDNSTokenProvider(name, raw)
	if err != nil {
		writeJSON(w, http.StatusOK, dnsTokenTestResponse{Error: err.Error()})
		return
	}
	zones, err := p.ListZones(r.Context())
	if err != nil {
		rt.logger.Warn("api: dns provider test failed", slog.String("provider", name), slog.String("error", err.Error()))
		writeJSON(w, http.StatusOK, dnsTokenTestResponse{Error: err.Error()})
		return
	}
	names := make([]string, 0, len(zones))
	for _, z := range zones {
		names = append(names, z.Name)
	}
	writeJSON(w, http.StatusOK, dnsTokenTestResponse{OK: true, Zones: names})
}

func (rt *Router) newDNSTokenProvider(name, credential string) (dnszones.Provider, error) {
	if rt.dnsTokenProvider != nil {
		return rt.dnsTokenProvider(name, credential)
	}
	p, err := dnszones.NewTokenProvider(name, credential)
	if err != nil {
		return nil, fmt.Errorf("api: %s client: %w", name, err)
	}
	return p, nil
}

// dnsTokenCredential returns the stored credential when the provider is enabled, else "".
func (rt *Router) dnsTokenCredential(ctx context.Context, name string) (string, error) {
	if rt.dnsTokenSecrets == nil || rt.dnsTokenStore == nil {
		return "", nil
	}
	enabled, err := rt.dnsTokenStore.GetDNSTokenProviderEnabled(ctx, name)
	if err != nil {
		return "", fmt.Errorf("api: get %s settings: %w", name, err)
	}
	if !enabled {
		return "", nil
	}
	cred, err := rt.dnsTokenSecrets.Resolve(ctx, store.DNSTokenSecretsKey(name), store.DNSTokenEnvKey)
	if err != nil {
		return "", fmt.Errorf("api: resolve %s credential: %w", name, err)
	}
	return cred, nil
}

// resolveDNSTokenRecordManager returns the first enabled token provider's record manager and its name.
func (rt *Router) resolveDNSTokenRecordManager(ctx context.Context) (dnsrecords.Manager, string, error) {
	for _, name := range dnszones.TokenProviders {
		cred, err := rt.dnsTokenCredential(ctx, name)
		if err != nil {
			return nil, "", err
		}
		if cred == "" {
			continue
		}
		p, err := rt.newDNSTokenProvider(name, cred)
		if err != nil {
			return nil, "", err
		}
		return dnsrecords.ZoneManager{P: p}, name, nil
	}
	return nil, "", nil
}

// addDNSTokenZoneProviders adds every enabled token provider to out.
func (rt *Router) addDNSTokenZoneProviders(ctx context.Context, out map[string]dnszones.Provider) error {
	for _, name := range dnszones.TokenProviders {
		cred, err := rt.dnsTokenCredential(ctx, name)
		if err != nil {
			return err
		}
		if cred == "" {
			continue
		}
		p, err := rt.newDNSTokenProvider(name, cred)
		if err != nil {
			return err
		}
		out[name] = p
	}
	return nil
}
