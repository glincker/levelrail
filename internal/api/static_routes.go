package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxStaticRoutesBody = 1 << 20

// staticRoutesResponse is a static site's effective routing rules. Published
// is false for a site deployed before routing rules existed.
type staticRoutesResponse struct {
	staticroutes.Ruleset
	Published bool `json:"published"`
}

type staticRoutesTestRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query"`
}

type staticRoutesLintRequest struct {
	Redirects string `json:"redirects"`
	Headers   string `json:"headers"`
}

type staticRoutesLintResponse struct {
	staticroutes.Ruleset
	Error string `json:"error,omitempty"`
}

// loadStaticSiteRules finds the live deploy's ruleset. A false ok means the
// response was already written.
func (rt *Router) loadStaticSiteRules(w http.ResponseWriter, r *http.Request) (site *store.StaticSite, rs *staticroutes.Ruleset, published, ok bool) {
	getter, can := rt.staticSites.(staticSiteGetter)
	if !can {
		writeError(w, http.StatusNotImplemented, "static site rules are not configured on this control plane")
		return nil, nil, false, false
	}
	name := r.PathValue("name")
	site, err := getter.GetStaticSite(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "static site not found")
		return nil, nil, false, false
	}
	if err != nil {
		rt.internalError(w, "api: static routes: lookup", err, slog.String("name", name))
		return nil, nil, false, false
	}
	rs, err = staticroutes.LoadSidecar(site.RootDir)
	if err != nil {
		rt.internalError(w, "api: static routes: load rules", err, slog.String("name", name))
		return nil, nil, false, false
	}
	if rs == nil {
		empty, cerr := staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits())
		if cerr != nil {
			rt.internalError(w, "api: static routes: empty rules", cerr)
			return nil, nil, false, false
		}
		return site, empty, false, true
	}
	return site, rs, true, true
}

// handleGetStaticRoutes handles GET /api/v1/apps/{name}/static-routes.
func (rt *Router) handleGetStaticRoutes(w http.ResponseWriter, r *http.Request) {
	_, rs, published, ok := rt.loadStaticSiteRules(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, staticRoutesResponse{Ruleset: *rs, Published: published})
}

// handleTestStaticRoutes handles POST /api/v1/apps/{name}/static-routes/test:
// it runs one request through the live rules and files and reports the match.
func (rt *Router) handleTestStaticRoutes(w http.ResponseWriter, r *http.Request) {
	var req staticRoutesTestRequest
	if err := decodeJSONBodyLimit(r, &req, maxStaticRoutesBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !strings.HasPrefix(req.Path, "/") || len(req.Path) > 2048 {
		writeError(w, http.StatusBadRequest, "path must start with / and be at most 2048 bytes")
		return
	}
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	site, rs, _, ok := rt.loadStaticSiteRules(w, r)
	if !ok {
		return
	}
	root, err := os.OpenRoot(site.RootDir)
	if err != nil {
		writeError(w, http.StatusGone, "the published files of this site are no longer on disk")
		return
	}
	defer root.Close() //nolint:errcheck // read-only handle
	d := rs.Resolve(root, staticroutes.Request{Method: method, Path: req.Path, RawQuery: strings.TrimPrefix(req.Query, "?")})
	writeJSON(w, http.StatusOK, d)
}

// handleLintStaticRoutes handles POST /api/v1/apps/{name}/static-routes/lint:
// it parses pasted _redirects and _headers text without deploying anything.
func (rt *Router) handleLintStaticRoutes(w http.ResponseWriter, r *http.Request) {
	var req staticRoutesLintRequest
	if err := decodeJSONBodyLimit(r, &req, maxStaticRoutesBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rs, err := staticroutes.Compile(staticroutes.Input{Redirects: req.Redirects, Headers: req.Headers}, staticroutes.LimitsFromEnv(os.LookupEnv))
	if err != nil {
		empty, _ := staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits())
		writeJSON(w, http.StatusOK, staticRoutesLintResponse{Ruleset: *empty, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, staticRoutesLintResponse{Ruleset: *rs})
}
