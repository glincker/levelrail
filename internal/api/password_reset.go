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

// passwordResetTokenTTL is how long a forgot-password link stays valid.
// 30 minutes: long enough that an operator who has to go find the email
// (a different device, a slow inbox sync) isn't racing a clock, short
// enough that a link sitting in an inbox or forwarded somewhere doesn't
// stay a usable credential for long. This sits at the upper end of the
// common 15-30 minute industry range for exactly this kind of link,
// rather than the middle of it, because this platform's admin account
// holds AbilityRoot over real infrastructure: a shorter window is worth
// the small inconvenience.
const passwordResetTokenTTL = 30 * time.Minute

// passwordResetSendTimeout bounds handleForgotPassword's background send
// (see that handler's own doc comment for why it runs in the
// background): long enough for a real SMTP/SES round trip, short enough
// that a hung backend can't leak a goroutine indefinitely.
const passwordResetSendTimeout = 30 * time.Second

// passwordResetTokenIDPrefix mirrors randomTokenID/randomBackupTargetID's
// own short, greppable-prefix convention.
const passwordResetTokenIDPrefix = "prt_"

func randomPasswordResetTokenID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate password reset token id: %w", err)
	}
	return passwordResetTokenIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// forgotPasswordLimiterKey scopes the rate limit to the client IP alone,
// unlike loginLimiterKey's (IP, username) pair: handleForgotPassword's
// request body carries no username (there is exactly one admin account
// to reset, see that handler's own doc comment on why that changes the
// threat model), so there is no second dimension to key on.
func forgotPasswordLimiterKey(r *http.Request) string {
	return "forgot-password|" + clientIP(r)
}

// handleForgotPassword handles POST /api/v1/auth/forgot-password: no
// request body is required, since this platform has exactly one admin
// account (unlike a typical multi-user "forgot password" endpoint, whose
// body carries an email to look up). That single-account property is
// also why the usual "don't leak whether this email has an account"
// concern barely applies here: reaching this API at all already implies
// exactly one target account exists. What must stay hidden instead is
// three different "how far did this get" states an attacker could
// otherwise probe for: no admin bootstrapped yet, an admin with no
// recovery email on file, and a real, working send. All three, along
// with a slow or failing SMTP/SES round trip, must look identical to the
// caller.
//
// The response is committed (rate-limit check aside, which depends only
// on public per-IP counters, never on account state) before any of that
// state is even read: the actual lookup-and-send runs in a detached
// background goroutine, so the response's timing carries no information
// about which of those states applies, a stronger guarantee than trying
// to pad every branch to the same duration. Rate limited per client IP
// (forgotPasswordLimiterKey): every call counts, not just "attempts that
// found something to send," since revealing that distinction through the
// backoff curve would defeat the point.
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

// sendPasswordResetEmail is handleForgotPassword's background half: look
// up the admin, generate and persist a reset token, and email it. Every
// early return here is a real, expected outcome (no admin yet, no
// recovery email on file, no email capability configured), logged at a
// level appropriate to whether it's an operator-actionable
// misconfiguration or just normal "nothing to do" state, never returned
// anywhere a caller could observe it: by the time this runs, the HTTP
// response has already been written.
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

// passwordResetURL builds the link the reset email points at. Prefers an
// absolute URL against the control plane's own configured primary domain
// (store.IngressSettings.PrimaryDomain) when one is set, since an email
// client needs an absolute link to be clickable at all; falls back to a
// bare path when it isn't, which still works for an operator who copies
// it onto whatever host they access the dashboard through.
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

// errInvalidOrExpiredResetToken is the single, generic rejection
// handleResetPassword returns for every way a token can fail: never
// found, expired, or already used. Distinguishing "this token was real
// but expired" from "this token never existed" would cost real
// complexity (a second, timing-safe code path) for a single-admin system
// where a distinguishable response gains an attacker very little (there
// is nothing to enumerate, only one account exists at all), so this
// collapses all three into one message and one status code, the same
// judgment call handleLogin's own doc comment already makes for
// username-vs-password.
var errInvalidOrExpiredResetToken = errors.New("invalid or expired reset token")

// handleResetPassword handles POST /api/v1/auth/reset-password:
// necessarily unauthenticated (the whole point), gated instead by
// possession of the token itself. On success, every existing session is
// revoked, with no exception (unlike handleChangePassword's
// revokeAllExcept): the requester here was never authenticated by a
// session in the first place, so there is no "current session" to leave
// alone.
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

	// Marking the token used comes after the password is already
	// changed: a failure here must never leave the operator's password
	// silently unchanged while claiming success, and must never be
	// allowed to block a reset that has, in every way that matters,
	// already happened. A failure here is logged, not surfaced: the
	// worst case is a token that stays technically valid a little
	// longer, not a security regression on par with the reset itself
	// failing silently.
	if err := rt.passwordResetTokens.MarkPasswordResetTokenUsed(r.Context(), rec.ID); err != nil {
		rt.logger.Error("api: reset password: mark token used failed", slog.String("error", err.Error()), slog.String("token_id", rec.ID))
	}

	rt.sessions.revokeAll(admin.Username)
	w.WriteHeader(http.StatusNoContent)
}
