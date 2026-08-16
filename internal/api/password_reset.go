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

// forgotPasswordIPKey and forgotPasswordEmailKey are two independent
// budgets, both must allow a request through: per-email (so one email
// being probed from many IPs still gets capped) and per-IP alone (so
// one IP can't bypass the per-email cap by cycling through a list of
// known/guessed emails, which would otherwise turn this endpoint into
// an unmetered email-bombing relay against arbitrary accounts).
func forgotPasswordIPKey(r *http.Request) string {
	return "forgot-password-ip|" + clientIP(r)
}

func forgotPasswordEmailKey(r *http.Request, email string) string {
	return "forgot-password-email|" + clientIP(r) + "|" + email
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// handleForgotPassword handles POST /api/v1/auth/forgot-password. The
// lookup and send run in a detached background goroutine after the
// response is already committed, so timing never reveals whether the
// email matched an account or whether the send succeeded.
func (rt *Router) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	ipKey := forgotPasswordIPKey(r)
	emailKey := forgotPasswordEmailKey(r, req.Email)
	okIP, retryAfterIP := rt.forgotPasswordByIP.allow(ipKey)
	okEmail, retryAfterEmail := rt.forgotPasswordByEmail.allow(emailKey)
	if !okIP || !okEmail {
		retryAfter := retryAfterIP
		if retryAfterEmail > retryAfter {
			retryAfter = retryAfterEmail
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many requests, try again later")
		return
	}
	rt.forgotPasswordByIP.recordFailure(ipKey)
	rt.forgotPasswordByEmail.recordFailure(emailKey)

	go rt.sendPasswordResetEmail(context.Background(), req.Email)

	w.WriteHeader(http.StatusNoContent)
}

// sendPasswordResetEmail is handleForgotPassword's background half.
// Every early return is a real, expected outcome, never surfaced to a
// caller: the HTTP response was already written by the time this runs.
func (rt *Router) sendPasswordResetEmail(ctx context.Context, email string) {
	ctx, cancel := context.WithTimeout(ctx, passwordResetSendTimeout)
	defer cancel()

	user, err := rt.auth.GetUserByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, store.ErrUserNotFound) {
			rt.logger.Error("api: forgot password: load user failed", slog.String("error", err.Error()))
		}
		return
	}
	if rt.emailSender == nil {
		rt.logger.Warn("api: forgot password: no email capability configured on this control plane")
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
		UserID:    user.ID,
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
		"A password reset was requested for your %s account.\n\nReset your password: %s\n\nThis link expires in %d minutes. If you didn't request this, you can safely ignore this email.",
		rt.brand.Name, url, int(passwordResetTokenTTL.Minutes()),
	)
	if err := rt.emailSender.Send(ctx, user.Email, subject, body); err != nil {
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

	// Claimed before the password changes, not after: this is the single
	// atomic point two concurrent requests holding the same token race
	// on, so at most one can proceed past it (see ClaimPasswordResetToken's
	// own doc comment).
	if err := rt.passwordResetTokens.ClaimPasswordResetToken(r.Context(), rec.ID); errors.Is(err, store.ErrPasswordResetTokenAlreadyUsed) {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredResetToken.Error())
		return
	} else if err != nil {
		rt.logger.Error("api: reset password: claim token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		rt.logger.Error("api: reset password: hash new password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	newHashStr := string(newHash)
	if err := rt.auth.UpdateUserPasswordHash(r.Context(), rec.UserID, &newHashStr); err != nil {
		rt.logger.Error("api: reset password: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.sessions.revokeAll(rec.UserID)
	w.WriteHeader(http.StatusNoContent)
}
