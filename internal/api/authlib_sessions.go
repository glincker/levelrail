package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const libMailTimeout = 30 * time.Second

// attachAuthEngine wires the library engine into the router: sessions, user
// sync, tokens, device login, MFA and OAuth. Without a supplied engine it
// builds a default one over the store database.
func (rt *Router) attachAuthEngine(s Store) {
	db, ok := s.(*store.DB)
	if !ok {
		panic("api: the auth engine needs a *store.DB")
	}
	if rt.authEngine == nil {
		eng, err := authengine.New(db.DB, authengine.Config{
			BaseURL:        "http://localhost",
			Directory:      authengine.NewDirectory(db.DB),
			DeviceTokenTTL: DeviceTokenTTL(), DeviceCodeTTL: DeviceCodeTTL(),
			Sessions: authengine.SessionsHooks{Mail: &authengine.MailRelay{}},
		})
		if err != nil {
			panic(fmt.Sprintf("api: build auth engine: %v", err))
		}
		rt.authEngine = eng
	}
	eng := rt.authEngine
	rt.libSessions = eng.Sessions()
	rt.authLib = authLibState{tokens: eng, device: eng}
	if eng.OAuthEnabled() {
		rt.authLibOAuth = eng
	}
	m, err := authengine.NewMFA(eng, db.DB)
	if err != nil {
		panic(fmt.Sprintf("api: build auth mfa: %v", err))
	}
	rt.mfaLib = &authLibMFA{rt: rt, mfa: m, seam: legacyLoginSeam{rt: rt}}
	rt.sessions.lib = rt.libSessions
	rt.auth = &libSyncedAuth{AuthStore: rt.auth, sess: rt.libSessions, logger: rt.logger}
	if relay := rt.libSessions.Mail(); relay != nil {
		relay.SetSender(rt.sendLibMail)
	}
}

func toLibUser(u store.User) authengine.LegacyUser {
	lu := authengine.LegacyUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt}
	if u.PasswordHash != nil {
		lu.PasswordHash = *u.PasswordHash
	}
	return lu
}

// writeLibAuthError maps a library failure onto the legacy status and body.
func (rt *Router) writeLibAuthError(w http.ResponseWriter, context string, err error) {
	var le *authengine.Error
	if !errors.As(err, &le) {
		rt.internalError(w, context, err)
		return
	}
	switch le.Code {
	case authengine.CodeInvalidCredentials:
		writeError(w, http.StatusUnauthorized, "invalid credentials")
	case authengine.CodeRateLimited, authengine.CodeAccountLocked:
		secs := int(le.RetryAfter.Seconds())
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
	default:
		rt.internalError(w, context, err)
	}
}

// handleLibLogin is handleLogin's library-backed body: same request, same
// responses, with the library owning the throttle, lockout and hash check.
func (rt *Router) handleLibLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	token, userID, err := rt.libSessions.Login(r.Context(), authengine.LoginInput{
		Email: req.Email, Password: req.Password, UserAgent: r.UserAgent(), IP: clientIP(r),
	})
	if errors.Is(err, authengine.ErrSecondFactorRequired) {
		pending, perr := rt.mfaPending.create(userID)
		if perr != nil {
			rt.internalError(w, "api: login: create mfa pending token failed", perr, slog.String("user_id", userID))
			return
		}
		writeJSON(w, http.StatusOK, loginResponse{MFARequired: true, MFAToken: pending})
		return
	}
	if err != nil {
		if authengine.IsCode(err, authengine.CodeInvalidCredentials) {
			rt.recordFailedPassword(r, req.Email)
		}
		rt.writeLibAuthError(w, "api: login: library sign-in failed", err)
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.libSessions.Revoke(r.Context(), token)
		rt.internalError(w, "api: login: load user failed", err, slog.String("user_id", userID))
		return
	}
	if rt.pauseForDeviceApproval(w, r, *user, token, signInMethodPassword) {
		return
	}
	rt.finishLibSession(w, r, *user, token)
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}

func (rt *Router) finishLibSession(w http.ResponseWriter, r *http.Request, user store.User, token string) {
	if err := rt.auth.UpdateUserLastLogin(r.Context(), user.ID, time.Now()); err != nil {
		rt.logger.Warn("api: update last_login_at failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
	}
	setSessionCookie(w, r, token, time.Now().Add(rt.sessions.ttl))
	rt.noticeSignIn(r, user, token)
}

// establishLibSession is establishSession's library-backed body.
func (rt *Router) establishLibSession(w http.ResponseWriter, r *http.Request, user store.User) error {
	token, err := rt.libSessions.IssueSession(r.Context(), toLibUser(user), r.UserAgent(), clientIP(r))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	rt.finishLibSession(w, r, user, token)
	return nil
}

// libRegister finishes handleRegister after its shared pre-checks. An
// address-shaped username goes through the library setup-token gate; a plain
// username (which the library refuses to sign up) is created by the platform
// store and mirrored, still guarded by the setup token and first-user index.
func (rt *Router) libRegister(w http.ResponseWriter, r *http.Request, req registerRequest) {
	ctx := r.Context()
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		rt.internalError(w, "api: register: hash password failed", err)
		return
	}
	id, err := randomOpaqueID("user_")
	if err != nil {
		rt.internalError(w, "api: register: generate user id failed", err)
		return
	}
	hashStr := string(hash)
	user := store.User{
		ID: id, Email: req.Email, DisplayName: req.Email, PasswordHash: &hashStr,
		Abilities: []string{AbilityRoot}, IsFirstUser: true, CreatedAt: time.Now(),
	}

	token, engineID, err := rt.libSessions.Register(ctx, authengine.RegisterInput{
		Email: req.Email, Password: req.Password, UserAgent: r.UserAgent(), IP: clientIP(r),
	})
	viaLibrary := err == nil
	switch {
	case errors.Is(err, authengine.ErrNotAnAddress):
	case err != nil:
		rt.writeLibRegisterError(w, err)
		return
	}

	saveCtx := ctx
	if viaLibrary {
		saveCtx = withoutLibSync(ctx)
	}
	createErr := rt.auth.CreateUser(saveCtx, user)
	if createErr == nil && viaLibrary {
		createErr = rt.libSessions.LinkUser(ctx, user.ID, engineID)
	}
	if createErr != nil {
		if viaLibrary {
			_ = rt.libSessions.DeleteUser(ctx, engineID)
		}
		if errors.Is(createErr, store.ErrFirstUserExists) || errors.Is(createErr, store.ErrUserEmailExists) {
			writeError(w, http.StatusConflict, "an account already exists; sign in instead")
			return
		}
		rt.internalError(w, "api: register: save user failed", createErr)
		return
	}
	if !viaLibrary {
		if token, err = rt.libSessions.IssueSession(ctx, toLibUser(user), r.UserAgent(), clientIP(r)); err != nil {
			rt.internalError(w, "api: register: issue session failed", err)
			return
		}
	}
	if err := removeSetupToken(rt.dataDir); err != nil {
		rt.logger.Warn("api: register: remove setup token failed", slog.String("error", err.Error()))
	}
	rt.finishLibSession(w, r, user, token)
	writeJSON(w, http.StatusCreated, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}

func (rt *Router) writeLibRegisterError(w http.ResponseWriter, err error) {
	var le *authengine.Error
	if !errors.As(err, &le) {
		rt.internalError(w, "api: register: library sign-up failed", err)
		return
	}
	switch le.Code {
	case authengine.CodeWeakPassword:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLength))
	case authengine.CodeEmailTaken, authengine.CodeSignupClosed:
		writeError(w, http.StatusConflict, "an account already exists; sign in instead")
	case authengine.CodeSetupTokenInvalid:
		writeError(w, http.StatusForbidden, "a valid setup token is required; print it on the server with the setup-token subcommand")
	case authengine.CodeRateLimited, authengine.CodeAccountLocked:
		rt.writeLibAuthError(w, "api: register: throttled", err)
	default:
		rt.internalError(w, "api: register: library sign-up failed", err)
	}
}

// requestLibPasswordReset runs detached after the forgot-password response,
// so timing never reveals whether the address matched an account.
func (rt *Router) requestLibPasswordReset(email, ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), libMailTimeout)
	defer cancel()
	if rt.emailSender == nil {
		rt.logger.Warn("api: forgot password: no email capability configured on this control plane")
		return
	}
	if err := rt.libSessions.RequestReset(ctx, email, ip); err != nil {
		rt.logger.Warn("api: forgot password: library reset request failed", slog.String("error", err.Error()))
	}
}

// sendLibMail turns a library password reset email into the platform's own
// message and link; any other library mail is dropped.
func (rt *Router) sendLibMail(ctx context.Context, to, _, body string) error {
	token, ok := authengine.ResetTokenFromMail(body)
	if !ok || rt.emailSender == nil {
		return nil
	}
	subject, text := rt.passwordResetMessage(ctx, token)
	return rt.emailSender.Send(ctx, to, subject, text)
}

// resetLibPassword is handleResetPassword's library-backed body.
func (rt *Router) resetLibPassword(w http.ResponseWriter, r *http.Request, req resetPasswordRequest) {
	owner, err := rt.libSessions.ResetPassword(r.Context(), req.Token, req.NewPassword, clientIP(r))
	if err != nil {
		var le *authengine.Error
		switch {
		case errors.As(err, &le) && le.Code == authengine.CodeRateLimited:
			rt.writeLibAuthError(w, "api: reset password: throttled", err)
		case errors.As(err, &le) && le.Status >= 400 && le.Status < 500:
			writeError(w, http.StatusBadRequest, errInvalidOrExpiredResetToken.Error())
		default:
			rt.internalError(w, "api: reset password: library reset failed", err)
		}
		return
	}
	if owner == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		rt.internalError(w, "api: reset password: hash new password failed", err)
		return
	}
	hashStr := string(hash)
	if err := rt.auth.UpdateUserPasswordHash(withoutLibSync(r.Context()), owner, &hashStr); err != nil {
		rt.internalError(w, "api: reset password: save failed", err)
		return
	}
	rt.resetSignInTrust(r.Context(), r, owner, "password_reset")
	w.WriteHeader(http.StatusNoContent)
}

// mintLibSessionLink mints a library session link for a user principal.
func (rt *Router) mintLibSessionLink(w http.ResponseWriter, r *http.Request, userID string) {
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: mint session link: load user failed", err, slog.String("user_id", userID))
		return
	}
	token, err := rt.libSessions.MintLink(r.Context(), toLibUser(*user))
	if err != nil {
		rt.internalError(w, "api: mint session link: library mint failed", err, slog.String("user_id", userID))
		return
	}
	writeJSON(w, http.StatusCreated, mintSessionLinkResponse{Token: token, URL: rt.sessionLinkURL(r, token)})
}

// consumeLibSessionLink exchanges a library session link for a session.
func (rt *Router) consumeLibSessionLink(w http.ResponseWriter, r *http.Request, link string) {
	token, userID, ok, err := rt.libSessions.ConsumeLink(r.Context(), link, r.UserAgent(), clientIP(r))
	if err != nil {
		rt.internalError(w, "api: consume session link: library consume failed", err)
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredSessionLink.Error())
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.libSessions.Revoke(r.Context(), token)
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredSessionLink.Error())
		return
	}
	rt.finishLibSession(w, withSignInNoticeSuppressed(r), *user, token)
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}

// watchLibSession cancels a stream the moment its library session stops
// validating. The returned stop is always safe to call.
func (rt *Router) watchLibSession(r *http.Request, revoked func()) (stop func()) {
	if rt.libSessions == nil {
		return func() {}
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return func() {}
	}
	if _, ok := rt.libSessions.Lookup(r.Context(), c.Value); !ok {
		return func() {}
	}
	ctx, cancel := rt.libSessions.Watch(context.WithoutCancel(r.Context()), c.Value, authengine.StreamWatchInterval())
	go func() {
		<-ctx.Done()
		if context.Cause(ctx) != nil && !errors.Is(context.Cause(ctx), context.Canceled) {
			revoked()
		}
	}()
	return func() { cancel(context.Canceled) }
}
