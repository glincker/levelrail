package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/forms"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxFormNotifyChannels = 10

type formResource struct {
	ID               string       `json:"id"`
	App              string       `json:"app"`
	Name             string       `json:"name"`
	RedirectURL      string       `json:"redirect_url"`
	HoneypotField    string       `json:"honeypot_field"`
	NotifyChannelIDs []string     `json:"notify_channel_ids"`
	EndpointPath     string       `json:"endpoint_path"`
	Endpoint         string       `json:"endpoint"`
	Snippet          string       `json:"snippet"`
	Snippets         formSnippets `json:"snippets"`
	Schema           forms.Schema `json:"schema"`
	// EndpointSource is "domain", "public_url" or "local"; local endpoints only work from this machine.
	EndpointSource  string    `json:"endpoint_source"`
	Submissions     int       `json:"submissions"`
	SpamSubmissions int       `json:"spam_submissions"`
	CreatedAt       time.Time `json:"created_at"`
}

type formSnippets struct {
	HTML    string `json:"html"`
	Fetch   string `json:"fetch"`
	Minimal string `json:"minimal"`
}

const (
	endpointSourceDomain    = "domain"
	endpointSourcePublicURL = "public_url"
	endpointSourceLocal     = "local"
)

type formsListResponse struct {
	Forms         []formResource `json:"forms"`
	RetentionDays int            `json:"retention_days"`
}

type formRequest struct {
	Name             string   `json:"name"`
	RedirectURL      *string  `json:"redirect_url"`
	HoneypotField    *string  `json:"honeypot_field"`
	NotifyChannelIDs []string `json:"notify_channel_ids"`
}

func formEndpointPath(app, name string) string {
	return "/api/v1/forms/" + app + "/" + name
}

type formsBase struct{ URL, Source string }

// formsBase picks the origin public posts should target: the app's own https domain, else
// the configured dashboard URL, else this request's host (flagged local, it may be a bare IP).
func (rt *Router) formsBase(r *http.Request, app string) formsBase {
	if svc, err := rt.apps.GetDesiredService(r.Context(), app); err == nil && svc != nil {
		for _, d := range svc.Domains {
			if d = strings.TrimSpace(d); d != "" && !strings.HasPrefix(d, "*") {
				return formsBase{URL: "https://" + d, Source: endpointSourceDomain}
			}
		}
	}
	if base := rt.dashboardURL(); base != "" {
		return formsBase{URL: strings.TrimRight(base, "/"), Source: endpointSourcePublicURL}
	}
	scheme := "https://"
	if host := strings.Split(r.Host, ":")[0]; host == "localhost" || host == "127.0.0.1" {
		scheme = "http://"
	}
	return formsBase{URL: scheme + r.Host, Source: endpointSourceLocal}
}

func (rt *Router) formView(f store.Form, c store.FormCounts, base formsBase) formResource {
	path := formEndpointPath(f.AppName, f.Name)
	endpoint := base.URL + path
	schema, _ := forms.ParseSchema(f.Schema)
	return formResource{
		ID: f.ID, App: f.AppName, Name: f.Name, RedirectURL: f.RedirectURL, HoneypotField: f.HoneypotField,
		NotifyChannelIDs: f.NotifyChannelIDs, EndpointPath: path, Endpoint: endpoint,
		Snippet:  forms.Snippet(endpoint, f),
		Snippets: formSnippets{HTML: forms.Snippet(endpoint, f), Fetch: forms.SnippetFetch(endpoint, f), Minimal: forms.SnippetMinimal(endpoint, f)},
		Schema:   schema, EndpointSource: base.Source,
		Submissions: c.Total, SpamSubmissions: c.Spam, CreatedAt: f.CreatedAt,
	}
}

func (rt *Router) formsAvailable(w http.ResponseWriter) bool {
	if rt.forms == nil {
		writeError(w, http.StatusNotImplemented, "forms are not available on this control plane")
		return false
	}
	return true
}

// loadFormForApp resolves {name}/{form} and writes the 404 itself.
func (rt *Router) loadFormForApp(w http.ResponseWriter, r *http.Request) (store.Form, bool) {
	if !rt.formsAvailable(w) {
		return store.Form{}, false
	}
	f, err := rt.forms.GetForm(r.Context(), r.PathValue("name"), r.PathValue("form"))
	if errors.Is(err, store.ErrFormNotFound) {
		writeError(w, http.StatusNotFound, "form not found")
		return store.Form{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load form", err, slog.String("app", r.PathValue("name")))
		return store.Form{}, false
	}
	return f, true
}

func (rt *Router) appExists(w http.ResponseWriter, r *http.Request, app string) bool {
	_, err := rt.apps.GetDesiredService(r.Context(), app)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return false
	}
	if err != nil {
		rt.internalError(w, "api: forms: load app", err, slog.String("name", app))
		return false
	}
	return true
}

func (rt *Router) handleListForms(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("name")
	if !rt.formsAvailable(w) || !rt.appExists(w, r, app) {
		return
	}
	list, counts, err := rt.forms.ListForms(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: list forms", err, slog.String("app", app))
		return
	}
	base := rt.formsBase(r, app)
	out := make([]formResource, 0, len(list))
	for _, f := range list {
		out = append(out, rt.formView(f, counts[f.ID], base))
	}
	writeJSON(w, http.StatusOK, formsListResponse{Forms: out, RetentionDays: int(rt.formsRuntime().cfg.Retention / (24 * time.Hour))})
}

func (rt *Router) handleCreateForm(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("name")
	if !rt.formsAvailable(w) {
		return
	}
	var req formRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := forms.ValidateName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f := store.Form{AppName: app, Name: req.Name, HoneypotField: forms.DefaultHoneypot}
	if !rt.applyFormRequest(w, r, &f, req) || !rt.appExists(w, r, app) {
		return
	}
	existing, _, err := rt.forms.ListForms(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: create form: count", err, slog.String("app", app))
		return
	}
	if limit := rt.formsRuntime().cfg.MaxFormsPerApp; limit > 0 && len(existing) >= limit {
		writeError(w, http.StatusConflict, "this app has reached its form limit")
		return
	}
	var idErr, saltErr error
	f.ID, idErr = forms.NewID()
	f.IPSalt, saltErr = forms.NewSalt()
	if err := errors.Join(idErr, saltErr); err != nil {
		rt.internalError(w, "api: create form: ids", err)
		return
	}
	f.CreatedAt = time.Now().UTC()
	switch err := rt.forms.CreateForm(r.Context(), f); {
	case errors.Is(err, store.ErrFormExists):
		writeError(w, http.StatusConflict, "a form with that name already exists")
	case err != nil:
		rt.internalError(w, "api: create form", err, slog.String("app", app))
	default:
		writeJSON(w, http.StatusCreated, rt.formView(f, store.FormCounts{}, rt.formsBase(r, app)))
	}
}

func (rt *Router) handleUpdateForm(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	var req formRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.applyFormRequest(w, r, &f, req) {
		return
	}
	if err := rt.forms.UpdateForm(r.Context(), f); err != nil {
		rt.internalError(w, "api: update form", err, slog.String("form_id", f.ID))
		return
	}
	writeJSON(w, http.StatusOK, rt.formView(f, store.FormCounts{}, rt.formsBase(r, f.AppName)))
}

// applyFormRequest copies the editable fields of req onto f, writing the 400 itself on bad input.
func (rt *Router) applyFormRequest(w http.ResponseWriter, r *http.Request, f *store.Form, req formRequest) bool {
	if req.RedirectURL != nil {
		if err := forms.ValidateRedirect(*req.RedirectURL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return false
		}
		f.RedirectURL = *req.RedirectURL
	}
	if req.HoneypotField != nil {
		if err := forms.ValidateHoneypot(*req.HoneypotField); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return false
		}
		f.HoneypotField = *req.HoneypotField
		if f.HoneypotField == "" {
			f.HoneypotField = forms.DefaultHoneypot
		}
	}
	if req.NotifyChannelIDs != nil {
		ids, ok := rt.checkFormChannels(r.Context(), w, req.NotifyChannelIDs)
		if !ok {
			return false
		}
		f.NotifyChannelIDs = ids
	}
	return true
}

func (rt *Router) checkFormChannels(ctx context.Context, w http.ResponseWriter, in []string) ([]string, bool) {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, id := range in {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return out, true
	}
	if len(out) > maxFormNotifyChannels {
		writeError(w, http.StatusBadRequest, "too many notification channels")
		return nil, false
	}
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return nil, false
	}
	for _, id := range out {
		if _, err := rt.notificationChannels.GetNotificationChannel(ctx, id); err != nil {
			writeError(w, http.StatusBadRequest, "unknown notification channel "+id)
			return nil, false
		}
	}
	return out, true
}

func (rt *Router) handleDeleteForm(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	if err := rt.forms.DeleteForm(r.Context(), f.ID); err != nil && !errors.Is(err, store.ErrFormNotFound) {
		rt.internalError(w, "api: delete form", err, slog.String("form_id", f.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
