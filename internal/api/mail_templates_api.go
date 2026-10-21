package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

const mailTestTimeout = 15 * time.Second

func (rt *Router) requireMail(w http.ResponseWriter) bool {
	if rt.mail.db == nil {
		writeError(w, http.StatusNotImplemented, "email templates are not available on this control plane")
		return false
	}
	return true
}

type mailTemplateView struct {
	mailtemplates.Event
	Current    mailtemplates.Template `json:"current"`
	Customized bool                   `json:"customized"`
	UpdatedAt  string                 `json:"updated_at,omitempty"`
}

func (rt *Router) templateView(ev mailtemplates.Event, override *store.MailTemplate) mailTemplateView {
	v := mailTemplateView{Event: ev, Current: ev.Default}
	if override != nil {
		cand := mailtemplates.Template{Subject: override.Subject, Preheader: override.Preheader, Body: override.Body}
		if mailtemplates.ValidateTemplate(ev, cand) == nil {
			v.Current, v.Customized, v.UpdatedAt = cand, true, override.UpdatedAt
		}
	}
	return v
}

// handleListMailTokens handles GET /api/v1/mail/tokens.
func (rt *Router) handleListMailTokens(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tokens": mailtemplates.Tokens()})
}

// handleListMailTemplates handles GET /api/v1/mail/templates.
func (rt *Router) handleListMailTemplates(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	overrides, err := rt.mail.db.ListMailTemplates(r.Context())
	if err != nil {
		rt.internalError(w, "api: list mail templates failed", err)
		return
	}
	byKey := map[string]*store.MailTemplate{}
	for i := range overrides {
		byKey[overrides[i].EventKey] = &overrides[i]
	}
	out := []mailTemplateView{}
	for _, ev := range mailtemplates.Events() {
		out = append(out, rt.templateView(ev, byKey[ev.Key]))
	}
	layout, err := rt.mail.db.GetMailLayout(r.Context())
	if err != nil {
		rt.internalError(w, "api: get mail layout failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out, "layout": layout})
}

func (rt *Router) loadTemplateView(w http.ResponseWriter, r *http.Request) (mailTemplateView, bool) {
	ev, ok := mailtemplates.LookupEvent(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such email template")
		return mailTemplateView{}, false
	}
	override, err := rt.mail.db.GetMailTemplate(r.Context(), ev.Key)
	if err != nil && !errors.Is(err, store.ErrMailNotFound) {
		rt.internalError(w, "api: get mail template failed", err)
		return mailTemplateView{}, false
	}
	return rt.templateView(ev, override), true
}

// handleGetMailTemplate handles GET /api/v1/mail/templates/{key}.
func (rt *Router) handleGetMailTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	if v, ok := rt.loadTemplateView(w, r); ok {
		writeJSON(w, http.StatusOK, v)
	}
}

// handleSetMailTemplate handles PUT /api/v1/mail/templates/{key}.
func (rt *Router) handleSetMailTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	ev, ok := mailtemplates.LookupEvent(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such email template")
		return
	}
	var req mailtemplates.Template
	if !decodeReq(w, r, &req) {
		return
	}
	if err := mailtemplates.ValidateTemplate(ev, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := rt.mail.db.SaveMailTemplate(r.Context(), store.MailTemplate{EventKey: ev.Key, Subject: req.Subject, Preheader: req.Preheader, Body: req.Body, UpdatedBy: rt.actorForRequest(r)})
	if err != nil {
		rt.internalError(w, "api: save mail template failed", err)
		return
	}
	if v, ok := rt.loadTemplateView(w, r); ok {
		writeJSON(w, http.StatusOK, v)
	}
}

// handleResetMailTemplate handles DELETE /api/v1/mail/templates/{key}: back to the built-in default.
func (rt *Router) handleResetMailTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	key := r.PathValue("key")
	if _, ok := mailtemplates.LookupEvent(key); !ok {
		writeError(w, http.StatusNotFound, "no such email template")
		return
	}
	if err := rt.mail.db.DeleteMailTemplate(r.Context(), key); err != nil {
		rt.internalError(w, "api: reset mail template failed", err)
		return
	}
	if v, ok := rt.loadTemplateView(w, r); ok {
		writeJSON(w, http.StatusOK, v)
	}
}

// handleListMailTemplateVersions handles GET /api/v1/mail/templates/{key}/versions.
func (rt *Router) handleListMailTemplateVersions(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	vs, err := rt.mail.db.ListMailTemplateVersions(r.Context(), r.PathValue("key"))
	if err != nil {
		rt.internalError(w, "api: list mail template versions failed", err)
		return
	}
	if vs == nil {
		vs = []store.MailTemplateVersion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": vs})
}

// handleRestoreMailTemplateVersion handles POST /api/v1/mail/templates/{key}/versions/{version}/restore.
func (rt *Router) handleRestoreMailTemplateVersion(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	ev, ok := mailtemplates.LookupEvent(r.PathValue("key"))
	n, convErr := strconv.Atoi(r.PathValue("version"))
	if !ok || convErr != nil {
		writeError(w, http.StatusNotFound, "no such email template version")
		return
	}
	v, err := rt.mail.db.GetMailTemplateVersion(r.Context(), ev.Key, n)
	if errors.Is(err, store.ErrMailNotFound) {
		writeError(w, http.StatusNotFound, "no such email template version")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get mail template version failed", err)
		return
	}
	cand := mailtemplates.Template{Subject: v.Subject, Preheader: v.Preheader, Body: v.Body}
	if err := mailtemplates.ValidateTemplate(ev, cand); err != nil {
		writeError(w, http.StatusBadRequest, "that version no longer validates: "+err.Error())
		return
	}
	if err := rt.mail.db.SaveMailTemplate(r.Context(), store.MailTemplate{EventKey: ev.Key, Subject: v.Subject, Preheader: v.Preheader, Body: v.Body, UpdatedBy: rt.actorForRequest(r)}); err != nil {
		rt.internalError(w, "api: restore mail template failed", err)
		return
	}
	if view, ok := rt.loadTemplateView(w, r); ok {
		writeJSON(w, http.StatusOK, view)
	}
}

// mailLayoutRequest is the PUT /api/v1/mail/layout body.
type mailLayoutRequest struct {
	HeaderImageURL string `json:"header_image_url"`
	FooterText     string `json:"footer_text"`
}

// handleSetMailLayout handles PUT /api/v1/mail/layout.
func (rt *Router) handleSetMailLayout(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	var req mailLayoutRequest
	if !decodeReq(w, r, &req) {
		return
	}
	l := mailtemplates.Layout{HeaderImageURL: strings.TrimSpace(req.HeaderImageURL), FooterText: req.FooterText}
	if err := mailtemplates.ValidateLayout(l); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.mail.db.SetMailLayout(r.Context(), store.MailLayout{HeaderImageURL: l.HeaderImageURL, FooterText: l.FooterText}); err != nil {
		rt.internalError(w, "api: set mail layout failed", err)
		return
	}
	writeJSON(w, http.StatusOK, store.MailLayout{HeaderImageURL: l.HeaderImageURL, FooterText: l.FooterText})
}

// mailPreviewRequest previews an unsaved draft; empty fields fall back to what is saved.
type mailPreviewRequest struct {
	Subject        *string `json:"subject"`
	Preheader      *string `json:"preheader"`
	Body           *string `json:"body"`
	HeaderImageURL *string `json:"header_image_url"`
	FooterText     *string `json:"footer_text"`
	Scheme         string  `json:"scheme"`
}

// handlePreviewMailTemplate handles POST /api/v1/mail/templates/{key}/preview with sample data.
func (rt *Router) handlePreviewMailTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	ev, ok := mailtemplates.LookupEvent(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such email template")
		return
	}
	var req mailPreviewRequest
	if !decodeReq(w, r, &req) {
		return
	}
	tpl, _ := rt.effectiveTemplate(r.Context(), ev.Key)
	if req.Subject != nil {
		tpl.Subject = *req.Subject
	}
	if req.Preheader != nil {
		tpl.Preheader = *req.Preheader
	}
	if req.Body != nil {
		tpl.Body = *req.Body
	}
	if err := mailtemplates.ValidateTemplate(ev, tpl); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	layout := rt.mailLayout(r.Context())
	if req.HeaderImageURL != nil {
		layout.HeaderImageURL = *req.HeaderImageURL
	}
	if req.FooterText != nil {
		layout.FooterText = *req.FooterText
	}
	if err := mailtemplates.ValidateLayout(layout); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts := mailtemplates.Options{}
	if ev.Category == mailtemplates.CategoryAnnouncement {
		opts.UnsubscribeURL = rt.emailLinkBase(r.Context()) + "/mail/unsubscribe?t=preview"
	}
	out := mailtemplates.Render(rt.mailBranding(r.Context()), layout, tpl, mailtemplates.SampleValues(), opts)
	out.HTML = mailtemplates.ForceScheme(out.HTML, req.Scheme)
	writeJSON(w, http.StatusOK, out)
}

// handleTestMailTemplate handles POST /api/v1/mail/templates/{key}/test: sends the saved template to the caller only.
func (rt *Router) handleTestMailTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	ev, ok := mailtemplates.LookupEvent(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such email template")
		return
	}
	userID, found := rt.currentSessionUserID(r)
	if !found {
		writeError(w, http.StatusBadRequest, "sign in to the dashboard as a user to send a test to yourself")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: test mail: load user failed", err)
		return
	}
	if !rt.mailProviderConfigured(r.Context()) {
		writeCodedError(w, http.StatusConflict, "email_not_configured", "no email provider is configured")
		return
	}
	vals := mailtemplates.SampleValues()
	for k, v := range recipientValues(user.Email, user.DisplayName, "") {
		vals[k] = v
	}
	opts := mailtemplates.Options{}
	if ev.Category == mailtemplates.CategoryAnnouncement {
		opts.UnsubscribeURL = rt.unsubscribeURL(r.Context(), user.Email)
	}
	m, _ := rt.mailMessage(r.Context(), ev.Key, user.Email, vals, opts)
	m.Subject = "[Test] " + m.Subject
	ctx, cancel := context.WithTimeout(r.Context(), mailTestTimeout)
	defer cancel()
	if err := email.SendMessage(ctx, rt.emailSender, m); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("test email failed: %s", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sent_to": user.Email})
}
