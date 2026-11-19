package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

// staticSitePort and staticSitePendingTag match what the dashboard's drop
// publish creates: a static site has no container, so the app is a placeholder.
const (
	staticSitePort       = 80
	staticSitePendingTag = ":pending"
)

type createStaticSiteRequest struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains,omitempty"`
}

// handleCreateStaticSite handles POST /api/v1/static-sites. It creates the
// backing app through handleCreateApp so every create rule (names, quotas,
// domains, projects, team scoping) applies unchanged.
func (rt *Router) handleCreateStaticSite(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDecodeBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req createStaticSiteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	body, err := json.Marshal(appResource{
		Name:    req.Name,
		Image:   req.Name + staticSitePendingTag,
		Port:    staticSitePort,
		Domains: req.Domains,
	})
	if err != nil {
		rt.internalError(w, "api: create static site: encode", err)
		return
	}
	inner := r.Clone(r.Context())
	inner.Body = io.NopCloser(strings.NewReader(string(body)))
	inner.ContentLength = int64(len(body))

	rec := httptest.NewRecorder()
	rt.handleCreateApp(rec, inner)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}
	domains := req.Domains
	var created appResource
	if json.Unmarshal(rec.Body.Bytes(), &created) == nil && created.Domains != nil {
		domains = created.Domains
	}
	writeJSON(w, http.StatusCreated, staticSiteDetail{
		Name: req.Name, BuildType: "static", Domains: nonNilStrings(domains),
	})
}
