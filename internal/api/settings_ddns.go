package api

import (
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/ddns"
)

// SetDDNS enables /api/v1/settings/ddns; without it the routes answer 501.
func (rt *Router) SetDDNS(m *ddns.Manager) { rt.ddnsMgr = m }

// ddnsUpdateRequest is write-only for target: it is stored encrypted and never returned.
type ddnsUpdateRequest struct {
	Enabled           *bool     `json:"enabled"`
	Provider          *string   `json:"provider"`
	Domains           *string   `json:"domains"`
	Target            *string   `json:"target"`
	IntervalSecs      *int      `json:"interval_seconds"`
	ProbeURLs         *[]string `json:"probe_urls"`
	ProbeQuorum       *int      `json:"probe_quorum"`
	TimeoutSecs       *int      `json:"timeout_seconds"`
	MinUpdateGapSecs  *int      `json:"min_update_gap_seconds"`
	ReassertSecs      *int      `json:"reassert_seconds"`
	FailAlertAfter    *int      `json:"fail_alert_after"`
	AllowPrivate      *bool     `json:"allow_private"`
	AllowInsecureHTTP *bool     `json:"allow_insecure_http"`
}

func (rt *Router) handleGetDDNS(w http.ResponseWriter, r *http.Request) {
	if rt.ddnsMgr == nil {
		writeError(w, http.StatusNotImplemented, "dynamic DNS is not available on this instance")
		return
	}
	res, err := rt.ddnsMgr.Get(r.Context())
	if err != nil {
		rt.internalError(w, "api: get ddns failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handlePutDDNS(w http.ResponseWriter, r *http.Request) {
	if rt.ddnsMgr == nil {
		writeError(w, http.StatusNotImplemented, "dynamic DNS is not available on this instance")
		return
	}
	var req ddnsUpdateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := rt.ddnsMgr.Apply(r.Context(), ddns.Update(req))
	switch {
	case errors.Is(err, ddns.ErrNoSecrets):
		writeError(w, http.StatusNotImplemented, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleDDNSStatus(w http.ResponseWriter, _ *http.Request) {
	if rt.ddnsMgr == nil {
		writeError(w, http.StatusNotImplemented, "dynamic DNS is not available on this instance")
		return
	}
	writeJSON(w, http.StatusOK, rt.ddnsMgr.Status())
}
