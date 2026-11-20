package api

import (
	"encoding/json"
	"net/http"
)

type baseWildcardRequest struct {
	Confirm bool `json:"confirm"`
	Replace bool `json:"replace"`
}

type baseWildcardResponse struct {
	DryRun     bool            `json:"dry_run"`
	BaseDomain string          `json:"base_domain"`
	DNS        domainDNSResult `json:"dns"`
}

// handleBaseDomainWildcard handles POST
// /api/v1/settings/ingress/apps-base-domain/wildcard (root): previews, or with
// confirm creates, the *.<base> record pointing at this server. A foreign
// record is only replaced when replace is also true.
func (rt *Router) handleBaseDomainWildcard(w http.ResponseWriter, r *http.Request) {
	var req baseWildcardRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, goLiveBodyLimit)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	base := rt.appsBaseDomain(r.Context())
	if base == "" {
		writeError(w, http.StatusBadRequest, "set the apps base domain first")
		return
	}
	mode := dnsModePreview
	if req.Confirm {
		mode = dnsModeAuto
	}
	d, _ := rt.applyDomainDNS(r.Context(), r, "", "*."+base, dnsOpts{Mode: mode, Replace: req.Replace, CanWrite: rt.callerIsRoot(r)})
	writeJSON(w, http.StatusOK, baseWildcardResponse{DryRun: !req.Confirm, BaseDomain: base, DNS: d})
}
