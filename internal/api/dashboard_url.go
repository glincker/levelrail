package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

type dashboardURLResource struct {
	DashboardURL string `json:"dashboard_url"`
}

// insecureLoginResponse is the 403 body for a refused plain-HTTP sign-in.
type insecureLoginResponse struct {
	Error     string `json:"error"`
	SecureURL string `json:"secure_url"`
}

var errDashboardURLInvalid = errors.New("dashboard_url must be an absolute http:// or https:// URL with a host and no path, query, or fragment")

// normalizeDashboardURL validates raw and returns it without a trailing slash; "" stays "".
func normalizeDashboardURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errDashboardURLInvalid
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errDashboardURLInvalid
	}
	return u.Scheme + "://" + u.Host, nil
}

func (rt *Router) handleGetDashboardURL(w http.ResponseWriter, r *http.Request) {
	u, err := rt.ingressSettings.GetDashboardURL(r.Context())
	if err != nil {
		rt.internalError(w, "api: get dashboard url failed", err)
		return
	}
	writeJSON(w, http.StatusOK, dashboardURLResource{DashboardURL: u})
}

func (rt *Router) handleUpdateDashboardURL(w http.ResponseWriter, r *http.Request) {
	var req dashboardURLResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := normalizeDashboardURL(req.DashboardURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Saving an https URL from a plain-HTTP session would lock that operator
	// out on their next sign-in if the https URL doesn't actually work yet.
	if strings.HasPrefix(u, "https://") && !requestIsHTTPS(r) && !peerIsLoopback(r) && !rt.allowInsecureLogin {
		writeError(w, http.StatusConflict, "open the dashboard at "+u+" and save this setting from there, so sign-in over https is proven to work first")
		return
	}
	if err := rt.ingressSettings.SetDashboardURL(r.Context(), u); err != nil {
		rt.internalError(w, "api: set dashboard url failed", err)
		return
	}
	writeJSON(w, http.StatusOK, dashboardURLResource{DashboardURL: u})
}

// refuseInsecureLogin writes a 403 and returns true when r is a plain-HTTP
// sign-in attempt on an instance whose dashboard URL is https. A settings
// read failure fails open so a broken row can never lock every operator out.
func (rt *Router) refuseInsecureLogin(w http.ResponseWriter, r *http.Request) bool {
	if rt.allowInsecureLogin || requestIsHTTPS(r) {
		return false
	}
	u, err := rt.ingressSettings.GetDashboardURL(r.Context())
	if err != nil {
		rt.logger.Warn("api: dashboard url lookup failed, allowing sign-in", slog.String("error", err.Error()))
		return false
	}
	if !strings.HasPrefix(u, "https://") {
		return false
	}
	writeJSON(w, http.StatusForbidden, insecureLoginResponse{
		Error:     "sign-in over plain HTTP is disabled on this instance; sign in at " + u,
		SecureURL: u,
	})
	return true
}
