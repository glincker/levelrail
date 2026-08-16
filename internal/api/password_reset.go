package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// passwordResetTokenTTL: upper end of the common 15-30 minute range,
// since this account holds AbilityRoot over real infrastructure.
const passwordResetTokenTTL = 30 * time.Minute

// passwordResetSendTimeout bounds handleForgotPassword's background
// send, so a hung SMTP/SES backend can't leak a goroutine indefinitely.
const passwordResetSendTimeout = 30 * time.Second

const passwordResetTokenIDPrefix = "prt_"

func randomPasswordResetTokenID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate password reset token id: %w", err)
	}
	return passwordResetTokenIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// forgotPasswordLimiterKey scopes the rate limit to the client IP alone:
// unlike login, there's no username in the request to key on (exactly
// one admin account exists to reset).
func forgotPasswordLimiterKey(r *http.Request) string {
	return "forgot-password|" + clientIP(r)
}

// handleForgotPassword handles POST /api/v1/auth/forgot-password. No
// body is required: there's exactly one admin account. The lookup and
// send run in a detached background goroutine after the response is
// already committed, so timing never reveals admin/email/send state.
func (rt *Router) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	key := forgotPasswordLimiterKey(r)
	if ok, retryAfter := rt.forgotPassword.allow(key); !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many requests, try again later")
		return
	}
	rt.forgotPassword.recordFailure(key)

	go rt.sendPasswordResetEmail(context.Background())

	w.WriteHeader(http.StatusNoContent)
}

// sendPasswordResetEmail is handleForgotPassword's background half.
// Every early return is a real, expected outcome, never surfaced to a
// caller: the HTTP response was already written by the time this runs.
func (rt *Router) sendPasswordResetEmail(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, passwordResetSendTimeout)
	defer cancel()

	admin, err := rt.auth.GetAdminUser(ctx)
	if err != nil {
		if !errors.Is(err, store.ErrAdminNotFound) {
			rt.logger.Error("api: forgot password: load admin user failed", slog.String("error", err.Error()))
		}
		return
	}
	if admin.Email == "" {
		return
	}
	if rt.emailSender == nil {
		rt.logger.Warn("api: forgot password: no admin email capability configured on this control plane")
		return
	}

	plaintext, err := randomToken()
	if err != nil {
		rt.logger.Error("api: forgot password: generate token failed", slog.String("error", err.Error()))
		return
	}
	id, err := randomPasswordResetTokenID()
	if err != nil {
		rt.logger.Error("api: forgot password: generate token id failed", slog.String("error", err.Error()))
		return
	}

	now := time.Now().UTC()
	rec := store.PasswordResetToken{
		ID:        id,
		TokenHash: hashToken(plaintext),
		CreatedAt: now,
		ExpiresAt: now.Add(passwordResetTokenTTL),
	}
	if err := rt.passwordResetTokens.SavePasswordResetToken(ctx, rec); err != nil {
		rt.logger.Error("api: forgot password: save token failed", slog.String("error", err.Error()))
		return
	}

	url := rt.passwordResetURL(ctx, plaintext)
	subject := fmt.Sprintf("[%s] Reset your password", rt.brand.Name)
	body := fmt.Sprintf(
		"A password reset was requested for your %s admin account.\n\nReset your password: %s\n\nThis link expires in %d minutes. If you didn't request this, you can safely ignore this email.",
		rt.brand.Name, url, int(passwordResetTokenTTL.Minutes()),
	)
	if err := rt.emailSender.Send(ctx, admin.Email, subject, body); err != nil {
		rt.logger.Error("api: forgot password: send email failed", slog.String("error", err.Error()))
	}
}

// passwordResetURL builds the link the reset email points at: absolute
// against the primary domain when one is set (an email client needs a
// clickable absolute link), a bare path otherwise.
func (rt *Router) passwordResetURL(ctx context.Context, token string) string {
	const path = "/reset-password?token="
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil || settings.PrimaryDomain == "" {
		return path + token
	}
	return "https://" + settings.PrimaryDomain + path + token
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// errInvalidOrExpiredResetToken covers every way a token can fail (not
// found, expired, used): distinguishing them isn't worth the complexity
// for a single-admin system with nothing to enumerate.
var errInvalidOrExpiredResetToken = errors.New("invalid or expired reset token")

// handleResetPassword handles POST /api/v1/auth/reset-password:
// unauthenticated by necessity, gated by possession of the token itself.
// Revokes every session with no exception, since the requester was never
// authenticated by one in the first place.
func (rt *Router) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if len(req.NewPassword) < minPasswordLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("new password must be at least %d characters", minPasswordLength))
		return
	}

	rec, err := rt.passwordResetTokens.GetPasswordResetTokenByHash(r.Context(), hashToken(req.Token))
	if errors.Is(err, store.ErrPasswordResetTokenNotFound) {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredResetToken.Error())
		return
	}
	if err != nil {
		rt.logger.Error("api: reset password: load token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rec.UsedAt != nil || time.Now().After(rec.ExpiresAt) {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredResetToken.Error())
		return
	}

	admin, err := rt.auth.GetAdminUser(r.Context())
	if err != nil {
		rt.logger.Error("api: reset password: load admin user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		rt.logger.Error("api: reset password: hash new password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.auth.UpsertAdminUser(r.Context(), admin.Username, string(newHash)); err != nil {
		rt.logger.Error("api: reset password: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// After the password is already changed: a failure here just leaves
	// the token valid a little longer, logged but not surfaced.
	if err := rt.passwordResetTokens.MarkPasswordResetTokenUsed(r.Context(), rec.ID); err != nil {
		rt.logger.Error("api: reset password: mark token used failed", slog.String("error", err.Error()), slog.String("token_id", rec.ID))
	}

	rt.sessions.revokeAll(admin.Username)
	w.WriteHeader(http.StatusNoContent)
}
