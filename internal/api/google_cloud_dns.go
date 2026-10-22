package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/store"
)

const googleCloudDNSInternalError = "internal error"

const googleCloudDNSNoMasterKey = "google cloud dns is not configured on this control plane (no master key set)"

// GoogleCloudDNSSecrets is the surface the Cloud DNS handlers need from
// internal/secrets.Manager. Resolve reads the key back for record paths only.
type GoogleCloudDNSSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// googleCloudDNSResource is the wire shape of the settings routes. The
// service account JSON never appears in it, only the project and client email.
type googleCloudDNSResource struct {
	Enabled     bool   `json:"enabled"`
	ProjectID   string `json:"project_id,omitempty"`
	ClientEmail string `json:"client_email,omitempty"`
	HasKey      bool   `json:"has_key"`
}

type updateGoogleCloudDNSRequest struct {
	Enabled bool `json:"enabled"`
	// ServiceAccountJSON empty keeps the stored key.
	ServiceAccountJSON string `json:"service_account_json,omitempty"`
}

type googleCloudDNSTestResponse struct {
	OK    bool     `json:"ok"`
	Zones []string `json:"zones,omitempty"`
	Error string   `json:"error,omitempty"`
}

func (rt *Router) toGoogleCloudDNSResource(ctx context.Context, s store.GoogleCloudDNSSettings) googleCloudDNSResource {
	res := googleCloudDNSResource{Enabled: s.Enabled, ProjectID: s.ProjectID, ClientEmail: s.ClientEmail}
	if rt.googleCloudDNSSecrets != nil {
		has, err := rt.googleCloudDNSSecrets.Exists(ctx, store.GoogleCloudDNSSecretsKey(), store.GoogleCloudDNSKeyEnvKey)
		if err != nil {
			rt.logger.Warn("api: check google cloud dns key failed", slog.String("error", err.Error()))
		}
		res.HasKey = has
	}
	return res
}

// handleGetGoogleCloudDNSSettings handles GET /api/v1/settings/google-cloud-dns.
func (rt *Router) handleGetGoogleCloudDNSSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.googleCloudDNS.GetGoogleCloudDNSSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get google cloud dns settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, googleCloudDNSInternalError)
		return
	}
	writeJSON(w, http.StatusOK, rt.toGoogleCloudDNSResource(r.Context(), settings))
}

// handleUpdateGoogleCloudDNSSettings handles PUT /api/v1/settings/google-cloud-dns.
func (rt *Router) handleUpdateGoogleCloudDNSSettings(w http.ResponseWriter, r *http.Request) {
	if rt.googleCloudDNSSecrets == nil {
		writeError(w, http.StatusNotImplemented, googleCloudDNSNoMasterKey)
		return
	}
	var req updateGoogleCloudDNSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	current, err := rt.googleCloudDNS.GetGoogleCloudDNSSettings(ctx)
	if err != nil {
		rt.logger.Error("api: get google cloud dns settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, googleCloudDNSInternalError)
		return
	}
	next := store.GoogleCloudDNSSettings{Enabled: req.Enabled, ProjectID: current.ProjectID, ClientEmail: current.ClientEmail}

	key := store.GoogleCloudDNSSecretsKey()
	if req.ServiceAccountJSON != "" {
		id, err := dnszones.ParseGoogleCloudKey(req.ServiceAccountJSON)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := rt.googleCloudDNSSecrets.SetValue(ctx, key, store.GoogleCloudDNSKeyEnvKey, req.ServiceAccountJSON); err != nil {
			rt.logger.Error("api: save google cloud dns key failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, googleCloudDNSInternalError)
			return
		}
		next.ProjectID, next.ClientEmail = id.ProjectID, id.ClientEmail
	} else if req.Enabled {
		has, err := rt.googleCloudDNSSecrets.Exists(ctx, key, store.GoogleCloudDNSKeyEnvKey)
		if err != nil {
			rt.logger.Error("api: check google cloud dns key failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, googleCloudDNSInternalError)
			return
		}
		if !has {
			writeError(w, http.StatusBadRequest, "a service account JSON key is required the first time google cloud dns is enabled")
			return
		}
	}
	if err := rt.googleCloudDNS.UpdateGoogleCloudDNSSettings(ctx, next); err != nil {
		rt.logger.Error("api: update google cloud dns settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, googleCloudDNSInternalError)
		return
	}
	writeJSON(w, http.StatusOK, rt.toGoogleCloudDNSResource(ctx, next))
}

// handleDisconnectGoogleCloudDNS handles DELETE /api/v1/settings/google-cloud-dns.
func (rt *Router) handleDisconnectGoogleCloudDNS(w http.ResponseWriter, r *http.Request) {
	if rt.googleCloudDNSSecrets == nil {
		writeError(w, http.StatusNotImplemented, googleCloudDNSNoMasterKey)
		return
	}
	if err := rt.googleCloudDNSSecrets.DeleteAll(r.Context(), store.GoogleCloudDNSSecretsKey()); err != nil {
		rt.logger.Error("api: clear google cloud dns key failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, googleCloudDNSInternalError)
		return
	}
	settings := store.GoogleCloudDNSSettings{}
	if err := rt.googleCloudDNS.UpdateGoogleCloudDNSSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: disconnect google cloud dns failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, googleCloudDNSInternalError)
		return
	}
	writeJSON(w, http.StatusOK, rt.toGoogleCloudDNSResource(r.Context(), settings))
}

// handleTestGoogleCloudDNS handles POST /api/v1/settings/google-cloud-dns/test:
// lists the project's managed zones with the stored key. A provider failure
// is reported in the body (ok=false), not as an HTTP error.
func (rt *Router) handleTestGoogleCloudDNS(w http.ResponseWriter, r *http.Request) {
	if rt.googleCloudDNSSecrets == nil {
		writeError(w, http.StatusNotImplemented, googleCloudDNSNoMasterKey)
		return
	}
	raw, err := rt.googleCloudDNSSecrets.Resolve(r.Context(), store.GoogleCloudDNSSecretsKey(), store.GoogleCloudDNSKeyEnvKey)
	if err != nil || raw == "" {
		writeJSON(w, http.StatusOK, googleCloudDNSTestResponse{Error: "no service account key is stored yet"})
		return
	}
	p, err := rt.newGoogleCloudProvider(raw)
	if err != nil {
		writeJSON(w, http.StatusOK, googleCloudDNSTestResponse{Error: err.Error()})
		return
	}
	zones, err := p.ListZones(r.Context())
	if err != nil {
		rt.logger.Warn("api: google cloud dns test failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusOK, googleCloudDNSTestResponse{Error: err.Error()})
		return
	}
	names := make([]string, 0, len(zones))
	for _, z := range zones {
		names = append(names, z.Name)
	}
	writeJSON(w, http.StatusOK, googleCloudDNSTestResponse{OK: true, Zones: names})
}

func (rt *Router) newGoogleCloudProvider(raw string) (dnszones.Provider, error) {
	if rt.googleCloudDNSProvider != nil {
		return rt.googleCloudDNSProvider(raw)
	}
	p, err := dnszones.NewGoogleCloud(raw)
	if err != nil {
		return nil, fmt.Errorf("api: google cloud dns client: %w", err)
	}
	return p, nil
}

// googleCloudDNSKey returns the stored key when Cloud DNS is enabled, else "".
func (rt *Router) googleCloudDNSKey(ctx context.Context) (string, error) {
	if rt.googleCloudDNSSecrets == nil || rt.googleCloudDNS == nil {
		return "", nil
	}
	s, err := rt.googleCloudDNS.GetGoogleCloudDNSSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("api: get google cloud dns settings: %w", err)
	}
	if !s.Enabled {
		return "", nil
	}
	key, err := rt.googleCloudDNSSecrets.Resolve(ctx, store.GoogleCloudDNSSecretsKey(), store.GoogleCloudDNSKeyEnvKey)
	if err != nil {
		return "", fmt.Errorf("api: resolve google cloud dns key: %w", err)
	}
	return key, nil
}

func (rt *Router) resolveGoogleCloudRecordManager(ctx context.Context) (dnsrecords.Manager, error) {
	key, err := rt.googleCloudDNSKey(ctx)
	if err != nil || key == "" {
		return nil, err
	}
	p, err := rt.newGoogleCloudProvider(key)
	if err != nil {
		return nil, err
	}
	return dnsrecords.ZoneManager{P: p}, nil
}
