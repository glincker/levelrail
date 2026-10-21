package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	emailChangeTTL      = time.Hour
	emailChangeSendWait = 15 * time.Second
	emailChangeLimit    = 3
	emailChangeWindow   = time.Hour
	maxEmailLen         = 254
)

type emailChangeStatus struct {
	Available bool `json:"available"`
}

type emailChangeRequestBody struct {
	NewEmail string `json:"new_email"`
	stepUpProof
}

type emailChangeConfirmBody struct {
	Token string `json:"token"`
}

// handleEmailChangeStatus handles GET /api/v1/auth/email-change: whether this
// server can send the verification mail at all, so the UI can offer a Fix.
func (rt *Router) handleEmailChangeStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.accountSessionCaller(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, emailChangeStatus{Available: rt.emailReady(r.Context()) && rt.accountDB() != nil})
}

// handleRequestEmailChange handles POST /api/v1/auth/email-change: after a
// password check it mails a link to the new address. The answer is the same
// whether or not that address is taken, so it cannot be used to probe accounts.
func (rt *Router) handleRequestEmailChange(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.accountSessionCaller(w, r)
	if !ok {
		return
	}
	db := rt.accountDB()
	if db == nil || !rt.emailReady(r.Context()) {
		writeCodedError(w, http.StatusConflict, "email_not_configured", "this server cannot send email yet: configure email first")
		return
	}
	var req emailChangeRequestBody
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	newEmail, valid := normalizeNewEmail(req.NewEmail)
	if !valid {
		writeError(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	if req.empty() {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "confirm your password to change your email")
		return
	}
	if err := rt.verifyPasswordProof(r, userID, req.stepUpProof); err != nil {
		writeStepUpError(w, err)
		return
	}
	if !rt.accountSelf.takeEmailChange(userID, time.Now()) {
		w.Header().Set("Retry-After", strconv.Itoa(int(emailChangeWindow.Seconds())))
		writeCodedError(w, http.StatusTooManyRequests, "email_change_limited", "too many email change requests: try again in an hour")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: email change: load user failed", err)
		return
	}
	_, lookupErr := rt.auth.GetUserByEmail(r.Context(), newEmail)
	unavailable := lookupErr == nil || strings.EqualFold(user.Email, newEmail)
	token, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: email change: token failed", err)
		return
	}
	id, err := randomTokenID()
	if err != nil {
		rt.internalError(w, "api: email change: id failed", err)
		return
	}
	now := time.Now()
	if err := db.CreateEmailChangeRequest(r.Context(), store.EmailChangeRequest{
		ID: id, UserID: userID, NewEmail: newEmail, TokenHash: hashToken(token), CreatedAt: now, ExpiresAt: now.Add(emailChangeTTL),
	}); err != nil {
		rt.internalError(w, "api: email change: save failed", err)
		return
	}
	if unavailable {
		rt.recordAccountAction(r, userID, true, accountEventEmailRequested, http.StatusAccepted)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	link := rt.emailLinkBase(r.Context()) + "/settings/account#confirm_email=" + token
	go rt.sendAccountEvent(context.WithoutCancel(r.Context()), mailtemplates.EventEmailChangeConfirm, newEmail, map[string]string{
		"confirm_link": link, "expires_in": fmt.Sprintf("%d minutes", int(emailChangeTTL.Minutes())),
	})
	rt.recordAccountAction(r, userID, true, accountEventEmailRequested, http.StatusAccepted)
	w.WriteHeader(http.StatusAccepted)
}

// handleConfirmEmailChange handles POST /api/v1/auth/email-change/confirm: the
// signed-in owner of a pending change completes it with the mailed token.
func (rt *Router) handleConfirmEmailChange(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.accountSessionCaller(w, r)
	if !ok {
		return
	}
	db := rt.accountDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "email change is not available on this server")
		return
	}
	var req emailChangeConfirmBody
	if err := decodeJSONBody(r, &req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	old, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: email confirm: load user failed", err)
		return
	}
	newEmail, err := db.ConsumeEmailChange(r.Context(), hashToken(strings.TrimSpace(req.Token)), userID, time.Now())
	if errors.Is(err, store.ErrEmailChangeInvalid) {
		writeCodedError(w, http.StatusBadRequest, "invalid_token", "this confirmation link is invalid, expired or already used")
		return
	}
	if errors.Is(err, store.ErrUserEmailExists) {
		writeCodedError(w, http.StatusConflict, "email_taken", "that address now belongs to another account")
		return
	}
	if err != nil {
		rt.internalError(w, "api: email confirm failed", err)
		return
	}
	if updated, err := rt.auth.GetUserByID(r.Context(), userID); err == nil && rt.libSessions != nil {
		if _, err := rt.libSessions.SyncUser(r.Context(), toLibUser(*updated)); err != nil {
			rt.logger.Warn("api: email change: sync to auth library failed", slog.String("user_id", userID), slog.String("error", err.Error()))
		}
	}
	rt.sendAccountEvent(context.WithoutCancel(r.Context()), mailtemplates.EventEmailChanged, old.Email, map[string]string{"new_email": newEmail})
	rt.recordAccountAction(r, userID, true, accountEventEmailChanged, http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]string{"email": newEmail})
}

func (rt *Router) sendAccountEvent(ctx context.Context, key, to string, values map[string]string) {
	if !rt.emailReady(ctx) || !strings.Contains(to, "@") {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, emailChangeSendWait)
	defer cancel()
	if err := rt.sendMailEvent(ctx, key, to, values); err != nil {
		rt.logger.Error("api: account email send failed", slog.String("error", err.Error()))
	}
}

func normalizeNewEmail(raw string) (string, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || len(raw) > maxEmailLen {
		return "", false
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw {
		return "", false
	}
	return raw, true
}
