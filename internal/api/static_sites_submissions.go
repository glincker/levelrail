package api

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"github.com/glincker/levelrail/internal/store"
)

// siteSubmissionResource is one stored post tagged with the form it came from.
type siteSubmissionResource struct {
	Form string `json:"form"`
	formSubmissionResource
}

type siteSubmissionsResponse struct {
	Submissions []siteSubmissionResource `json:"submissions"`
}

// handleListStaticSiteSubmissions merges the newest submissions of every form on a site.
// The caller needs read-sensitive access because fields hold visitor input.
func (rt *Router) handleListStaticSiteSubmissions(w http.ResponseWriter, r *http.Request) {
	site := r.PathValue("name")
	if !rt.formsAvailable(w) || !rt.siteOrAppExists(w, r, site) {
		return
	}
	limit := formSubmissionsDefaultLimit
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, formSubmissionsMaxLimit)
	}
	fl, _, err := rt.forms.ListForms(r.Context(), site)
	if err != nil {
		rt.internalError(w, "api: list site submissions: forms", err, slog.String("site", site))
		return
	}
	out := make([]siteSubmissionResource, 0, limit)
	q := parseSubmissionQuery(r, limit)
	q.BeforeID = 0
	for _, f := range fl {
		rows, err := rt.forms.ListFormSubmissions(r.Context(), f.ID, q)
		if err != nil {
			rt.internalError(w, "api: list site submissions", err, slog.String("form_id", f.ID))
			return
		}
		for _, s := range rows {
			out = append(out, siteSubmissionResource{Form: f.Name, formSubmissionResource: submissionView(s)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	writeJSON(w, http.StatusOK, siteSubmissionsResponse{Submissions: out})
}

// handleDeleteStaticSiteSubmission removes one submission of a form on the site.
func (rt *Router) handleDeleteStaticSiteSubmission(w http.ResponseWriter, r *http.Request) {
	if !rt.formsAvailable(w) {
		return
	}
	f, err := rt.forms.GetForm(r.Context(), r.PathValue("name"), r.PathValue("form"))
	if err != nil {
		rt.finishFormLookup(w, err)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid submission id")
		return
	}
	err = rt.forms.DeleteFormSubmission(r.Context(), f.ID, id)
	if err == nil {
		rt.auditSubmissionDelete(r, f, id)
	}
	rt.finishSubmissionWrite(w, f, err)
}

func (rt *Router) finishFormLookup(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrFormNotFound) {
		writeError(w, http.StatusNotFound, "form not found")
		return
	}
	rt.internalError(w, "api: site submission: load form", err)
}

// siteOrAppExists accepts a static site that has no desired_services row, so
// existing sites list an empty set instead of an error.
func (rt *Router) siteOrAppExists(w http.ResponseWriter, r *http.Request, name string) bool {
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
		return true
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: site submissions: load app", err, slog.String("name", name))
		return false
	}
	if g, ok := rt.staticSites.(staticSiteGetter); ok {
		if _, err := g.GetStaticSite(r.Context(), name); err == nil {
			return true
		} else if !errors.Is(err, store.ErrServiceNotFound) {
			rt.internalError(w, "api: site submissions: load site", err, slog.String("name", name))
			return false
		}
	}
	writeError(w, http.StatusNotFound, "site not found")
	return false
}
