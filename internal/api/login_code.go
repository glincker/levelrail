package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

type loginCodeRequest struct {
	Username string `json:"username"`
}

type loginCodeRequestResponse struct {
	Message   string `json:"message"`
	ExpiresIn int    `json:"expires_in"`
}

type loginCodeRedeemRequest struct {
	Code string `json:"code"`
}

// handleRequestLoginCode handles POST /api/v1/auth/login-code/request. The
// response, status and minimum duration are the same whether or not the
// account exists or may use codes; an ineligible request stores a decoy.
func (rt *Router) handleRequestLoginCode(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	var req loginCodeRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if !rt.allowLoginCodeRequest(w, r, username) {
		return
	}
	ctx := r.Context()
	user, eligible := rt.loginCodeTarget(ctx, username)
	code, err := newLoginCode()
	if err != nil {
		rt.internalError(w, "api: login code request: generate code failed", err)
		return
	}
	salt, err := newSalt()
	if err != nil {
		rt.internalError(w, "api: login code request: generate salt failed", err)
		return
	}
	binding, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: login code request: generate binding failed", err)
		return
	}
	id, err := randomOpaqueID("lc_")
	if err != nil {
		rt.internalError(w, "api: login code request: generate id failed", err)
		return
	}
	st := rt.codeLogin
	now := time.Now()
	c := store.LoginCodeChallenge{
		ID: id, BrowserHash: hashToken(binding), CodeHash: hashLoginCode(salt, code), Salt: salt,
		RequesterIP: clientIP(r), UserAgent: truncateUA(r.UserAgent()), CreatedAt: now, ExpiresAt: now.Add(st.ttl),
	}
	if eligible {
		c.UserID = user.ID
	}
	if err := rt.loginCodes.CreateLoginCodeChallenge(ctx, c); err != nil {
		rt.internalError(w, "api: login code request: save challenge failed", err)
		return
	}
	rt.auditSignIn(ctx, r, anonymousSignIn(c.UserID), store.AuditActionLoginCodeRequest, loginCodeAuditPrefix+c.ID, http.StatusAccepted)
	if eligible {
		st.remember(c.ID, pendingPlainCode{userID: user.ID, code: code, expires: c.ExpiresAt})
		go rt.emailLoginCode(context.WithoutCancel(ctx), *user, c, code)
	}
	waitUntil(ctx, start.Add(st.minResponse))
	setBindingCookie(w, r, loginCodeCookie, loginCodeCookiePth, binding, st.ttl)
	writeJSON(w, http.StatusAccepted, loginCodeRequestResponse{Message: msgLoginCodeSent, ExpiresIn: int(st.ttl.Seconds())})
}

func waitUntil(ctx context.Context, deadline time.Time) {
	d := time.Until(deadline)
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

func truncateUA(ua string) string {
	const maxUA = 300
	if len(ua) > maxUA {
		return ua[:maxUA]
	}
	return ua
}

// allowLoginCodeRequest applies the global, per-IP and per-account budgets.
// The account key is the typed name, so it reveals nothing about existence.
func (rt *Router) allowLoginCodeRequest(w http.ResponseWriter, r *http.Request, username string) bool {
	st := rt.codeLogin
	checks := []struct {
		l   *apiRateLimiter
		key string
	}{
		{st.global, "global"},
		{st.byIP, clientIP(r)},
		{st.byAccount, strings.ToLower(username)},
	}
	for _, c := range checks {
		if ok, retry := c.l.allow(c.key); !ok {
			writeRateLimited(w, retry)
			return false
		}
	}
	return true
}

// loginCodeTarget resolves the account a code may be sent for. Any failure
// reads as ineligible so the caller's response never changes.
func (rt *Router) loginCodeTarget(ctx context.Context, username string) (*store.User, bool) {
	user, err := rt.auth.GetUserByEmail(ctx, username)
	if err != nil {
		if !errors.Is(err, store.ErrUserNotFound) {
			rt.logger.Warn("api: login code request: user lookup failed", slog.String("error", err.Error()))
		}
		return nil, false
	}
	allowed, err := rt.codeLoginAllowed(ctx, user)
	if err != nil {
		rt.logger.Warn("api: login code request: settings lookup failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
		return nil, false
	}
	return user, allowed
}

// emailLoginCode sends the code to the account's own address when mail is
// configured. Runs detached so the request's timing never depends on it.
func (rt *Router) emailLoginCode(parent context.Context, user store.User, c store.LoginCodeChallenge, code string) {
	ctx, cancel := context.WithTimeout(parent, libMailTimeout)
	defer cancel()
	channel := "dashboard"
	if rt.emailSender != nil {
		if _, perr := mail.ParseAddress(user.Email); perr == nil {
			subject, body := rt.loginCodeMessage(c, code)
			switch err := rt.emailSender.Send(ctx, user.Email, subject, body); {
			case err == nil:
				channel = "dashboard,email"
			case errors.Is(err, email.ErrNotConfigured):
			default:
				rt.logger.Warn("api: login code email failed", slog.String("user_id", user.ID), slog.String("error", err.Error()))
			}
		}
	}
	rt.auditSignIn(ctx, nil, anonymousSignIn(user.ID), store.AuditActionLoginCodeDeliver, loginCodeAuditPrefix+c.ID+"#"+channel, http.StatusOK)
}

func (rt *Router) loginCodeMessage(c store.LoginCodeChallenge, code string) (subject, body string) {
	subject = fmt.Sprintf("[%s] Your sign-in code", rt.brand.Name)
	body = fmt.Sprintf(
		"Someone asked to sign in to your %s account.\n\nCode: %s\n\nRequested from %s (%s) at %s. The code expires in %d minutes and only works in the browser that asked for it.\n\nIf this was not you, ignore this email and consider changing your password.",
		rt.brand.Name, formatLoginCode(code), c.RequesterIP, c.UserAgent, c.CreatedAt.UTC().Format(time.RFC1123),
		int(c.ExpiresAt.Sub(c.CreatedAt).Minutes()))
	return subject, body
}

// handleRedeemLoginCode handles POST /api/v1/auth/login-code/redeem: only
// the browser holding the challenge's binding cookie can redeem it. Every
// failure reads the same, so a guesser learns nothing about the account.
func (rt *Router) handleRedeemLoginCode(w http.ResponseWriter, r *http.Request) {
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	st := rt.codeLogin
	if ok, retry := st.redeemByIP.allow(clientIP(r)); !ok {
		writeRateLimited(w, retry)
		return
	}
	var req loginCodeRedeemRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	code := normalizeLoginCode(req.Code)
	browser, ok := cookieHash(r, loginCodeCookie)
	if !ok || len(code) != loginCodeLength {
		writeError(w, http.StatusUnauthorized, msgLoginCodeInvalid)
		return
	}
	ctx := r.Context()
	now := time.Now()
	c, err := rt.loginCodes.GetLiveLoginCodeByBrowser(ctx, browser, now)
	if errors.Is(err, store.ErrLoginCodeNotFound) {
		writeError(w, http.StatusUnauthorized, msgLoginCodeInvalid)
		return
	}
	if err != nil {
		rt.internalError(w, "api: login code redeem: load challenge failed", err)
		return
	}
	attempt, ok, err := rt.loginCodes.ReserveLoginCodeAttempt(ctx, c.ID, st.maxAttempts)
	if err != nil {
		rt.internalError(w, "api: login code redeem: reserve attempt failed", err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, msgLoginCodeInvalid)
		return
	}
	if c.UserID == "" || !loginCodeMatches(c, code) {
		rt.failLoginCode(ctx, r, c, attempt)
		writeError(w, http.StatusUnauthorized, msgLoginCodeInvalid)
		return
	}
	redeemed, err := rt.loginCodes.RedeemLoginCode(ctx, c.ID, now)
	if err != nil {
		rt.internalError(w, "api: login code redeem: mark redeemed failed", err)
		return
	}
	if !redeemed {
		writeError(w, http.StatusUnauthorized, msgLoginCodeInvalid)
		return
	}
	st.forget(c.ID)
	setBindingCookie(w, r, loginCodeCookie, loginCodeCookiePth, "", 0)
	user, err := rt.auth.GetUserByID(ctx, c.UserID)
	if err != nil {
		rt.internalError(w, "api: login code redeem: load user failed", err, slog.String("user_id", c.UserID))
		return
	}
	if allowed, aerr := rt.codeLoginAllowed(ctx, user); aerr != nil || !allowed {
		writeError(w, http.StatusUnauthorized, msgLoginCodeInvalid)
		return
	}
	rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionLoginCodeRedeem, loginCodeAuditPrefix+c.ID, http.StatusOK)
	rt.completeFirstFactor(w, r, *user)
}

func (rt *Router) failLoginCode(ctx context.Context, r *http.Request, c store.LoginCodeChallenge, attempt int) {
	path := loginCodeAuditPrefix + c.ID
	if attempt < rt.codeLogin.maxAttempts {
		rt.auditSignIn(ctx, r, anonymousSignIn(c.UserID), store.AuditActionLoginCodeFail, path, http.StatusUnauthorized)
		return
	}
	locked, err := rt.loginCodes.LockLoginCode(ctx, c.ID, time.Now())
	if err != nil {
		rt.logger.Warn("api: lock login code failed", slog.String("challenge_id", c.ID), slog.String("error", err.Error()))
	}
	rt.codeLogin.forget(c.ID)
	if locked {
		rt.auditSignIn(ctx, r, anonymousSignIn(c.UserID), store.AuditActionLoginCodeLockout, path, http.StatusUnauthorized)
	}
}

// completeFirstFactor finishes a sign-in whose first factor (a code, or an
// approved new device) checked out. TOTP, when enabled, is still required.
func (rt *Router) completeFirstFactor(w http.ResponseWriter, r *http.Request, user store.User) {
	totpOn, err := rt.totpEnabled(r.Context(), user.ID)
	if err != nil {
		rt.internalError(w, "api: sign-in: read two-factor status failed", err, slog.String("user_id", user.ID))
		return
	}
	if totpOn {
		pending, perr := rt.mfaPending.create(user.ID)
		if perr != nil {
			rt.internalError(w, "api: sign-in: create mfa pending token failed", perr, slog.String("user_id", user.ID))
			return
		}
		writeJSON(w, http.StatusOK, loginResponse{MFARequired: true, MFAToken: pending})
		return
	}
	if err := rt.establishSession(w, r, user); err != nil {
		rt.internalError(w, "api: sign-in: establish session failed", err, slog.String("user_id", user.ID))
		return
	}
	rt.trustDevice(w, r, user.ID)
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}

func (rt *Router) totpEnabled(ctx context.Context, userID string) (bool, error) {
	if rt.mfaLib == nil || rt.mfaLib.mfa == nil || !rt.mfaLib.mfa.TOTPAvailable() {
		return false, nil
	}
	st, err := rt.mfaLib.mfa.TOTPStatus(ctx, userID)
	if errors.Is(err, authengine.ErrNotMapped) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("api: totp status: %w", err)
	}
	return st.Enabled, nil
}
