package api

import (
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/forms"
	"github.com/GLINCKER/levelrail/internal/store"
)

const formThanksHTML = `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Received</title><p>Thanks, your submission was received.</p>`

// setFormsCORS opens the public endpoint to any site: it is write-only, takes no
// credentials and never returns stored data.
func setFormsCORS(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	h.Set("Access-Control-Max-Age", "600")
	h.Set("Access-Control-Expose-Headers", "Retry-After")
}

func (rt *Router) handleFormPreflight(w http.ResponseWriter, r *http.Request) {
	setFormsCORS(w)
	rt.narrowFormsCORS(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// narrowFormsCORS replaces the wildcard origin with the caller's when the form lists allowed origins.
func (rt *Router) narrowFormsCORS(w http.ResponseWriter, r *http.Request) {
	if rt.forms == nil {
		return
	}
	f, err := rt.forms.GetForm(r.Context(), r.PathValue("app"), r.PathValue("name"))
	if err != nil {
		return
	}
	s, _ := forms.ParseSchema(f.Schema)
	if len(s.AllowedOrigins) == 0 {
		return
	}
	w.Header().Add("Vary", "Origin")
	if origin := r.Header.Get("Origin"); formOriginAllowed(s, origin) && origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Del("Access-Control-Allow-Origin")
	}
}

// handleFormSubmit is the public capture endpoint. Unknown forms answer 404 and
// spam answers exactly like success, so a bot learns nothing from the response.
func (rt *Router) handleFormSubmit(w http.ResponseWriter, r *http.Request) {
	setFormsCORS(w)
	if rt.forms == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	fr := rt.formsRuntime()
	if ok, retry := fr.perIP.allow(clientIP(r)); !ok {
		formsRetryAfter(w, retry)
		return
	}
	f, err := rt.forms.GetForm(r.Context(), r.PathValue("app"), r.PathValue("name"))
	if errors.Is(err, store.ErrFormNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: form submit: load form", err)
		return
	}
	if ok, retry := fr.perForm.allow(f.ID); !ok {
		formsRetryAfter(w, retry)
		return
	}
	schema, _ := forms.ParseSchema(f.Schema)
	rt.narrowFormsCORS(w, r)
	if !formOriginAllowed(schema, r.Header.Get("Origin")) {
		writeError(w, http.StatusForbidden, "this site is not allowed to post to this form")
		return
	}
	fields, err := forms.ParseRequest(r, fr.cfg)
	if err != nil {
		writeError(w, formParseStatus(err), formParseMessage(err))
		return
	}
	fields, problems := forms.ValidateSubmission(schema, f.HoneypotField, fields)
	if problems != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "some fields are invalid", "fields": problems})
		return
	}
	if _, err := fr.svc.Submit(r.Context(), f, fields, clientIP(r)); err != nil {
		rt.internalError(w, "api: form submit", err, slog.String("form_id", f.ID))
		return
	}
	respondFormSuccess(w, r, f, schema)
}

func formsRetryAfter(w http.ResponseWriter, retry time.Duration) {
	secs := int(retry.Seconds()) + 1
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "too many submissions, try again later")
}

func formParseStatus(err error) int {
	switch {
	case errors.Is(err, forms.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, forms.ErrUnsupportedType):
		return http.StatusUnsupportedMediaType
	case errors.Is(err, forms.ErrTooManyFields), errors.Is(err, forms.ErrFieldTooLong):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}

func formParseMessage(err error) string {
	switch {
	case errors.Is(err, forms.ErrTooLarge):
		return "submission too large"
	case errors.Is(err, forms.ErrUnsupportedType):
		return "Content-Type must be form-urlencoded, multipart/form-data or application/json"
	case errors.Is(err, forms.ErrTooManyFields):
		return "too many fields"
	case errors.Is(err, forms.ErrFieldTooLong):
		return "a field is too long"
	case errors.Is(err, forms.ErrEmpty):
		return "no fields submitted"
	default:
		return "malformed submission"
	}
}

// respondFormSuccess redirects browser form posts to the form's redirect_url and answers API clients with JSON.
func respondFormSuccess(w http.ResponseWriter, r *http.Request, f store.Form, schema forms.Schema) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": schema.SuccessMessage})
		return
	}
	if f.RedirectURL != "" {
		http.Redirect(w, r, f.RedirectURL, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'none'")
	_, _ = w.Write([]byte(formThanksPage(schema.SuccessMessage)))
}

func wantsJSON(r *http.Request) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && mt == "application/json" {
		return true
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")
}
