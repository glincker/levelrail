package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envSignInAlertTTL       = "APP_SIGNIN_ALERT_LINK_TTL"
	envSignInAlertRate      = "APP_SIGNIN_ALERTS_PER_ACCOUNT_PER_HOUR"
	envKnownBrowserKeep     = "APP_KNOWN_BROWSER_RETENTION"
	defaultSignInAlertTTL   = 6 * time.Hour
	defaultSignInAlertRate  = 5
	defaultKnownBrowserKeep = 180 * 24 * time.Hour
	signInAlertPagePath     = "/sign-in-alert"
	signInAlertAuditPath    = "/api/v1/auth/sign-in-alert/"
	disownReason            = "sign_in_disowned"

	msgInvalidAlertLink = "this link is invalid, already used or expired"
)

type signInQuietKey struct{}

// withSignInNoticeSuppressed marks a sign-in the owner already vouched for
// (an approved browser, a session link), so it raises no new-browser alert.
func withSignInNoticeSuppressed(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), signInQuietKey{}, true))
}

// ipNetworkPrefix is the /24 (IPv4) or /48 (IPv6) an address belongs to,
// so a home router's address churn does not read as a new browser.
func ipNetworkPrefix(ip string) string {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	switch {
	case parsed == nil:
		return "unknown"
	case parsed.To4() != nil:
		return parsed.Mask(net.CIDRMask(24, 32)).String() + "/24"
	default:
		return parsed.Mask(net.CIDRMask(48, 128)).String() + "/48"
	}
}

func browserFingerprint(label, ip string) string {
	sum := sha256.Sum256([]byte(label + "\x00" + ipNetworkPrefix(ip)))
	return hex.EncodeToString(sum[:])
}

func (rt *Router) signAlertID(id string) string {
	m := hmac.New(sha256.New, rt.sec.alertKey)
	m.Write([]byte("sign-in-alert\x00" + id))
	return id + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// verifyAlertToken returns the alert id a link names when its signature holds.
func (rt *Router) verifyAlertToken(token string) (string, bool) {
	id, _, ok := strings.Cut(token, ".")
	if !ok || id == "" || len(token) > 256 {
		return "", false
	}
	want := rt.signAlertID(id)
	return id, subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
}

// knownBrowser reports whether r carries this account's trusted-device cookie.
func (rt *Router) knownBrowser(r *http.Request, userID string) bool {
	h, ok := cookieHash(r, trustedCookie)
	if !ok || rt.loginCodes == nil {
		return false
	}
	d, err := rt.loginCodes.GetTrustedDeviceByHash(r.Context(), h, time.Now())
	return err == nil && d.UserID == userID
}

// noticeSignIn remembers the browser a sign-in came from and, when the
// account has signed in before but never from this browser and network,
// emails the owner a "this wasn't me" link and tells opted-in channels.
func (rt *Router) noticeSignIn(r *http.Request, user store.User, token string) {
	if rt.security == nil {
		return
	}
	ctx := r.Context()
	if quiet, _ := ctx.Value(signInQuietKey{}).(bool); quiet {
		return
	}
	now := time.Now()
	label, ip := browserLabel(r.UserAgent()), clientIP(r)
	isNew, hadAny, err := rt.security.RememberBrowser(ctx, user.ID, browserFingerprint(label, ip), now)
	if err != nil {
		rt.logger.Warn("api: remember sign-in browser failed", slog.String("user_id", user.ID), slog.String("error", err.Error()))
		return
	}
	if !isNew || !hadAny || rt.knownBrowser(r, user.ID) {
		return
	}
	rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionNewBrowserSignIn, signInAlertAuditPath+safeSignInIP(ip), http.StatusOK)
	if ok, _ := rt.sec.alertLimit.allow(user.ID); !ok {
		return
	}
	sessionID := ""
	if rt.libSessions != nil && token != "" {
		sessionID, _ = rt.libSessions.SessionIDFor(ctx, token)
	}
	id, err := randomOpaqueID("sa_")
	if err != nil {
		rt.logger.Warn("api: sign-in alert id failed", slog.String("error", err.Error()))
		return
	}
	alert := store.SignInAlertToken{ID: id, UserID: user.ID, SessionID: sessionID, IP: ip, Label: label,
		CreatedAt: now, ExpiresAt: now.Add(envDuration(envSignInAlertTTL, defaultSignInAlertTTL))}
	if err := rt.security.CreateSignInAlertToken(ctx, alert); err != nil {
		rt.logger.Warn("api: save sign-in alert failed", slog.String("user_id", user.ID), slog.String("error", err.Error()))
		return
	}
	link := rt.emailLinkBase(ctx) + signInAlertPagePath + "?token=" + rt.signAlertID(id)
	rt.sendSecurityNotice(fmt.Sprintf("Security: account %s signed in from a new browser (%s at %s). If that was not expected, review sessions in the security center.",
		safeSignInText(user.Email), label, safeSignInIP(ip)))
	go rt.emailSignInAlert(context.WithoutCancel(ctx), user, alert, link)
}

// emailSignInAlert sends the owner the only copy of the "this wasn't me"
// link. Channels never get it: anyone reading a channel could use it.
func (rt *Router) emailSignInAlert(parent context.Context, user store.User, a store.SignInAlertToken, link string) {
	if rt.emailSender == nil {
		return
	}
	if _, err := mail.ParseAddress(user.Email); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, libMailTimeout)
	defer cancel()
	subject := fmt.Sprintf("[%s] New sign-in from %s", rt.brand.Name, a.Label)
	body := fmt.Sprintf("Your %s account signed in from a browser it has not used before.\n\nBrowser: %s\nAddress: %s\nTime: %s\n\n"+
		"If this was you, there is nothing to do.\n\nIf it was not you, open this link to end that session, forget your trusted browsers and flag the account for a new password:\n%s\n\nThe link works once and expires at %s.",
		rt.brand.Name, a.Label, safeSignInIP(a.IP), a.CreatedAt.UTC().Format(time.RFC1123), link, a.ExpiresAt.UTC().Format(time.RFC1123))
	if err := rt.emailSender.Send(ctx, user.Email, subject, body); err != nil && !errors.Is(err, email.ErrNotConfigured) {
		rt.logger.Warn("api: sign-in alert email failed", slog.String("user_id", user.ID), slog.String("error", err.Error()))
	}
}

type disownSignInRequest struct {
	Token string `json:"token"`
}

type disownSignInResponse struct {
	SessionRevoked bool `json:"session_revoked"`
}

// handleDisownSignIn handles POST /api/v1/auth/sign-in-alert/disown: public,
// gated by the signed single-use link. It ends the alerted session, drops
// every trusted browser of the account and flags it for a password reset.
func (rt *Router) handleDisownSignIn(w http.ResponseWriter, r *http.Request) {
	if refuseUnsafeSignInPOST(w, r) {
		return
	}
	if ok, retry := rt.codeLogin.redeemByIP.allow("disown|" + clientIP(r)); !ok {
		writeRateLimited(w, retry)
		return
	}
	var req disownSignInRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	id, ok := rt.verifyAlertToken(strings.TrimSpace(req.Token))
	if !ok {
		writeError(w, http.StatusBadRequest, msgInvalidAlertLink)
		return
	}
	ctx := r.Context()
	now := time.Now()
	a, err := rt.security.ConsumeSignInAlertToken(ctx, id, now)
	if errors.Is(err, store.ErrSignInAlertTokenUsed) {
		writeError(w, http.StatusBadRequest, msgInvalidAlertLink)
		return
	}
	if err != nil {
		rt.internalError(w, "api: disown sign-in: consume failed", err)
		return
	}
	// The link is spent by now, so every step runs even if an earlier one
	// failed; the request still reports the failure.
	out := disownSignInResponse{}
	var failed error
	if err := rt.security.FlagUserForReset(ctx, a.UserID, disownReason, now); err != nil {
		failed = fmt.Errorf("flag account: %w", err)
	}
	if a.SessionID != "" && rt.libSessions != nil {
		revoked, rerr := rt.libSessions.RevokeUserSession(ctx, a.UserID, a.SessionID)
		if rerr != nil {
			failed = fmt.Errorf("revoke session: %w", rerr)
		}
		out.SessionRevoked = revoked
	}
	defer func() {
		if failed != nil {
			rt.internalError(w, "api: disown sign-in failed, sign out other sessions from the dashboard", failed, slog.String("user_id", a.UserID))
			return
		}
		writeJSON(w, http.StatusOK, out)
	}()
	if err := rt.security.ForgetBrowser(ctx, a.UserID, browserFingerprint(a.Label, a.IP)); err != nil {
		rt.logger.Warn("api: disown sign-in: forget browser failed", slog.String("user_id", a.UserID), slog.String("error", err.Error()))
	}
	rt.revokeTrustedDevices(ctx, r, anonymousSignIn(a.UserID), a.UserID, disownReason)
	rt.retireSignInRequests(ctx, r, a.UserID, disownReason)
	rt.auditSignIn(ctx, r, anonymousSignIn(a.UserID), store.AuditActionSignInDisowned, signInAlertAuditPath+a.ID, http.StatusOK)
	rt.sec.posture.invalidate()
	rt.sendSecurityNotice("Security: the owner of an account reported a sign-in from " + safeSignInIP(a.IP) + " as not theirs. That session was ended and the account is flagged for a new password.")
}
