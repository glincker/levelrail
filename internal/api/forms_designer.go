package api

import (
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/forms"
	"github.com/GLINCKER/levelrail/internal/store"
)

type formTestResponse struct {
	SubmissionID int64             `json:"submission_id"`
	Spam         bool              `json:"spam"`
	SpamReason   string            `json:"spam_reason,omitempty"`
	Fields       map[string]string `json:"fields"`
}

func (rt *Router) handleGetFormFields(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	s, err := forms.ParseSchema(f.Schema)
	if err != nil {
		rt.internalError(w, "api: get form fields", err, slog.String("form_id", f.ID))
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (rt *Router) handleSetFormFields(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	var s forms.Schema
	if err := decodeJSONBody(r, &s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := forms.ValidateSchema(s, f.HoneypotField); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, err := marshalSchema(s)
	if err != nil {
		rt.internalError(w, "api: set form fields", err, slog.String("form_id", f.ID))
		return
	}
	f.Schema = raw
	if err := rt.forms.UpdateForm(r.Context(), f); err != nil {
		rt.internalError(w, "api: set form fields", err, slog.String("form_id", f.ID))
		return
	}
	writeJSON(w, http.StatusOK, rt.formView(f, store.FormCounts{}, rt.formsBase(r, f.AppName)))
}

func (rt *Router) handleSendFormTest(w http.ResponseWriter, r *http.Request) {
	f, ok := rt.loadFormForApp(w, r)
	if !ok {
		return
	}
	s, _ := forms.ParseSchema(f.Schema)
	sub, err := rt.formsRuntime().svc.SubmitTest(r.Context(), f, forms.SampleValues(s))
	if err != nil {
		rt.internalError(w, "api: form test submission", err, slog.String("form_id", f.ID))
		return
	}
	writeJSON(w, http.StatusOK, formTestResponse{SubmissionID: sub.ID, Spam: sub.Spam, SpamReason: sub.SpamReason, Fields: sub.Fields})
}

// formOriginAllowed reports whether a browser post from origin may use the form. Posts without
// an Origin header (curl, server-side) pass: the allow-list guards browsers, not scripts.
func formOriginAllowed(s forms.Schema, origin string) bool {
	if len(s.AllowedOrigins) == 0 || origin == "" {
		return true
	}
	for _, o := range s.AllowedOrigins {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

func formThanksPage(msg string) string {
	if msg == "" {
		return formThanksHTML
	}
	return `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Received</title><p>` + html.EscapeString(msg) + `</p>`
}

func marshalSchema(s forms.Schema) (string, error) {
	if s.Fields == nil {
		s.Fields = []forms.Field{}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("api: marshal form schema: %w", err)
	}
	return string(b), nil
}
