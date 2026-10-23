package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/forms"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	formSubmissionsDefaultLimit = 50
	formSubmissionsMaxLimit     = 200
	formExportMaxRows           = 50000
)

type formSubmissionResource struct {
	ID         int64             `json:"id"`
	Fields     map[string]string `json:"fields"`
	Spam       bool              `json:"spam"`
	Test       bool              `json:"test,omitempty"`
	SpamReason string            `json:"spam_reason,omitempty"`
	IPHash     string            `json:"ip_hash,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

type formSubmissionsResponse struct {
	Submissions []formSubmissionResource `json:"submissions"`
	// NextBefore is the cursor for the next page, 0 when this was the last one.
	NextBefore int64 `json:"next_before"`
}

type formSpamRequest struct {
	Spam bool `json:"spam"`
}

func submissionView(s store.FormSubmission) formSubmissionResource {
	return formSubmissionResource{ID: s.ID, Fields: s.Fields, Spam: s.Spam, Test: s.Test, SpamReason: s.SpamReason, IPHash: s.IPHash, CreatedAt: s.CreatedAt}
}

// parseSubmissionQuery reads q, spam=true|false and before from the URL; limit is the page size.
func parseSubmissionQuery(r *http.Request, limit int) store.FormSubmissionQuery {
	v := r.URL.Query()
	q := store.FormSubmissionQuery{Search: v.Get("q"), Limit: limit}
	switch v.Get("spam") {
	case "true":
		t := true
		q.Spam = &t
	case "false":
		f := false
		q.Spam = &f
	}
	if n, err := strconv.ParseInt(v.Get("before"), 10, 64); err == nil && n > 0 {
		q.BeforeID = n
	}
	return q
}

func (rt *Router) handleListFormSubmissions(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	limit := formSubmissionsDefaultLimit
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, formSubmissionsMaxLimit)
	}
	rows, err := rt.forms.ListFormSubmissions(r.Context(), f.ID, parseSubmissionQuery(r, limit+1))
	if err != nil {
		rt.internalError(w, "api: list form submissions", err, slog.String("form_id", f.ID))
		return
	}
	resp := formSubmissionsResponse{Submissions: make([]formSubmissionResource, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		resp.NextBefore = rows[len(rows)-1].ID
	}
	for _, s := range rows {
		resp.Submissions = append(resp.Submissions, submissionView(s))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) handleExportFormSubmissions(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	rows, err := rt.forms.ListFormSubmissions(r.Context(), f.ID, parseSubmissionQuery(r, formExportMaxRows))
	if err != nil {
		rt.internalError(w, "api: export form submissions", err, slog.String("form_id", f.ID))
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+f.AppName+"-"+f.Name+`-submissions.csv"`)
	if err := forms.WriteCSV(w, rows); err != nil {
		rt.logger.Error("api: write form csv", slog.String("form_id", f.ID), slog.String("error", err.Error()))
	}
}

func (rt *Router) handleSetFormSubmissionSpam(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req formSpamRequest
	if err != nil || decodeJSONBody(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	reason := ""
	if req.Spam {
		reason = forms.SpamManual
	}
	rt.finishSubmissionWrite(w, f, rt.forms.SetFormSubmissionSpam(r.Context(), f.ID, id, req.Spam, reason))
}

func (rt *Router) handleDeleteFormSubmission(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid submission id")
		return
	}
	rt.finishSubmissionWrite(w, f, rt.forms.DeleteFormSubmission(r.Context(), f.ID, id))
}

func (rt *Router) finishSubmissionWrite(w http.ResponseWriter, f store.Form, err error) {
	switch {
	case errors.Is(err, store.ErrFormSubmissionNotFound):
		writeError(w, http.StatusNotFound, "submission not found")
	case err != nil:
		rt.internalError(w, "api: form submission write", err, slog.String("form_id", f.ID))
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
