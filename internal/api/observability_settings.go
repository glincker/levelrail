package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// prometheusRemoteReadPath is the address handlePrometheusRead serves,
// surfaced to the UI so an operator can copy it straight into Grafana's
// Prometheus data source config without reading this package's source.
const prometheusRemoteReadPath = "/api/v1/prometheus/read"

// observabilitySettingsResource is the wire shape for GET and PUT
// /api/v1/settings/observability. RemoteReadPath is server-generated,
// never accepted on PUT: it is a fact about this build, not operator
// config.
type observabilitySettingsResource struct {
	ExternalDashboardURL string `json:"external_dashboard_url"`
	RemoteReadPath       string `json:"remote_read_path"`
}

func toObservabilitySettingsResource(s store.ObservabilitySettings) observabilitySettingsResource {
	return observabilitySettingsResource{
		ExternalDashboardURL: s.ExternalDashboardURL,
		RemoteReadPath:       prometheusRemoteReadPath,
	}
}

// handleGetObservabilitySettings handles GET /api/v1/settings/observability.
func (rt *Router) handleGetObservabilitySettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.observability.GetObservabilitySettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get observability settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toObservabilitySettingsResource(settings))
}

var errExternalDashboardURLInvalid = errors.New("external_dashboard_url must be an absolute http:// or https:// URL with a host")

// normalizeExternalDashboardURL validates raw and returns it trimmed;
// "" stays "" (clearing the link). Deliberately looser than
// normalizeDashboardURL: a Grafana deep link legitimately carries a path,
// query, or fragment (e.g. a specific dashboard UID), unlike the login
// dashboard URL that function validates.
func normalizeExternalDashboardURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errExternalDashboardURLInvalid
	}
	return raw, nil
}

// handleUpdateObservabilitySettings handles PUT /api/v1/settings/observability.
func (rt *Router) handleUpdateObservabilitySettings(w http.ResponseWriter, r *http.Request) {
	var req observabilitySettingsResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := normalizeExternalDashboardURL(req.ExternalDashboardURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	settings := store.ObservabilitySettings{ExternalDashboardURL: u}
	if err := rt.observability.UpdateObservabilitySettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: update observability settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toObservabilitySettingsResource(settings))
}
