package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

type mailBroadcastTestRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// handleTestMailBroadcast handles POST /api/v1/mail/broadcasts/test: the draft, rendered as a recipient would see it, sent to the caller only.
func (rt *Router) handleTestMailBroadcast(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	var req mailBroadcastTestRequest
	if !decodeReq(w, r, &req) {
		return
	}
	ev, _ := mailtemplates.LookupEvent(mailtemplates.EventAnnouncement)
	if err := mailtemplates.ValidateTemplate(ev, mailtemplates.Template{Subject: req.Subject, Body: req.Body}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, found := rt.currentSessionUserID(r)
	if !found {
		writeError(w, http.StatusBadRequest, "sign in to the dashboard as a user to send a test to yourself")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: test broadcast: load user failed", err)
		return
	}
	if !rt.mailProviderConfigured(r.Context()) {
		writeCodedError(w, http.StatusConflict, "email_not_configured", "no email provider is configured")
		return
	}
	draft := store.MailBroadcast{Subject: "[Test] " + strings.TrimSpace(req.Subject), Body: req.Body}
	m, err := rt.renderBroadcastMessage(r.Context(), draft, store.MailRecipient{Email: user.Email, UserID: user.ID})
	if err != nil {
		rt.internalError(w, "api: test broadcast: render failed", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), mailTestTimeout)
	defer cancel()
	if err := rt.sendBroadcastMessage(ctx, m); err != nil {
		writeError(w, http.StatusBadGateway, "test email failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sent_to": user.Email})
}
