package authengine

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
	"github.com/glincker/theauth-go/v2/crypto"
)

// ErrNotAnAddress means the username is not an email address, which the
// library cannot sign up; sign-in with such a username still works.
var ErrNotAnAddress = errors.New("authengine: username is not an email address")

// ErrSecondFactorRequired means the password was right but the library holds a
// confirmed TOTP secret for the user, so only a pending session was issued.
var ErrSecondFactorRequired = errors.New("authengine: second factor required")

// LoginInput is one password login attempt.
type LoginInput struct {
	Email, Password, UserAgent, IP string
}

// Login verifies the password through the library (throttle, lockout, legacy
// bcrypt rehash) and returns the session token plus the platform user id.
func (s *Sessions) Login(ctx context.Context, in LoginInput) (token, legacyUserID string, err error) {
	if err := s.ensureSyncedByEmail(ctx, in.Email); err != nil {
		return "", "", err
	}
	rep, err := s.call(ctx, routeSignin, map[string]string{"email": in.Email, "password": in.Password}, callOpts{ip: in.IP, userAgent: in.UserAgent})
	if err != nil {
		return "", "", err
	}
	token = rep.cookieValue(cookieName)
	if token == "" {
		return "", "", errors.New("authengine: signin returned no session cookie")
	}
	info, ok := s.Lookup(ctx, token)
	if !ok {
		if _, uid, pending := s.lookupSession(ctx, token); pending {
			legacy, lerr := s.legacyID(ctx, uid.String())
			s.Revoke(ctx, token)
			if lerr == nil && legacy != "" {
				return "", legacy, ErrSecondFactorRequired
			}
		} else {
			s.Revoke(ctx, token)
		}
		return "", "", &Error{Status: 401, Code: CodeInvalidCredentials, Message: "invalid email or password"}
	}
	return token, info.LegacyUserID, nil
}

// RegisterInput is the first-run registration request.
type RegisterInput struct {
	Email, Password, UserAgent, IP string
}

// Register creates the first library user behind the library setup-token gate
// and returns its session token and library id. The host then creates the
// platform user and calls LinkUser, or DiscardUser to roll back.
func (s *Sessions) Register(ctx context.Context, in RegisterInput) (token, engineUserID string, err error) {
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	if a == nil {
		return "", "", errors.New("authengine: sessions engine not bound")
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(in.Email)); err != nil {
		return "", "", ErrNotAnAddress
	}
	pending := a.SetupToken()
	if pending == "" {
		return "", "", &Error{Status: 403, Code: CodeSignupClosed, Message: "signup is closed"}
	}
	rep, err := s.call(ctx, routeSignup, map[string]string{"email": in.Email, "password": in.Password}, callOpts{ip: in.IP, userAgent: in.UserAgent, setupToken: pending})
	if err != nil {
		return "", "", err
	}
	token = rep.cookieValue(cookieName)
	info, ok := s.lookupEngine(ctx, token)
	if !ok {
		return "", "", errors.New("authengine: signup returned no usable session")
	}
	return token, info, nil
}

func (s *Sessions) lookupEngine(ctx context.Context, token string) (string, bool) {
	s.mu.RLock()
	authn := s.authn
	s.mu.RUnlock()
	if authn == nil || token == "" {
		return "", false
	}
	slot := &sessionSlot{}
	req, err := newCookieRequest(ctx, slot, token)
	if err != nil {
		return "", false
	}
	authn.ServeHTTP(&libReply{header: map[string][]string{}}, req)
	if slot.user == nil {
		return "", false
	}
	return slot.user.ID.String(), true
}

// IssueSession starts a full session for an already authenticated platform user
// (second factor, session link, OAuth), creating the library user if needed.
func (s *Sessions) IssueSession(ctx context.Context, u LegacyUser, userAgent, ip string) (string, error) {
	engine, err := s.SyncUser(ctx, u)
	if err != nil {
		return "", err
	}
	id, err := parseULID(engine)
	if err != nil {
		return "", fmt.Errorf("authengine: parse engine user id: %w", err)
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	tok, err := a.IssueSessionByUserID(ctx, id, userAgent, hostOnly(ip))
	if err != nil {
		return "", fmt.Errorf("authengine: issue session: %w", err)
	}
	return tok, nil
}

// Revoke ends the session named by token. Unknown tokens are not an error.
func (s *Sessions) Revoke(ctx context.Context, token string) {
	if err := s.revokeToken(ctx, token); err != nil {
		s.logger.Warn("authengine: revoke session failed", "error", err.Error())
	}
}

// RevokeChecked is Revoke for a caller that must not continue when the
// session may still be live. Unknown tokens are not an error.
func (s *Sessions) RevokeChecked(ctx context.Context, token string) error {
	return s.revokeToken(ctx, token)
}

func (s *Sessions) revokeToken(ctx context.Context, token string) error {
	sid, uid, ok := s.lookupSession(ctx, token)
	if !ok {
		return nil
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	ctx = theauth.WithAuditMetadata(ctx, theauth.AuditMetadata{ActorUserID: &uid})
	if err := a.RevokeSession(ctx, sid); err != nil {
		return fmt.Errorf("authengine: revoke session: %w", err)
	}
	return nil
}

func (s *Sessions) lookupSession(ctx context.Context, token string) (sessionID, userID theauth.ULID, ok bool) {
	s.mu.RLock()
	authn := s.authn
	s.mu.RUnlock()
	if authn == nil || token == "" {
		return theauth.ULID{}, theauth.ULID{}, false
	}
	slot := &sessionSlot{}
	req, err := newCookieRequest(ctx, slot, token)
	if err != nil {
		return theauth.ULID{}, theauth.ULID{}, false
	}
	authn.ServeHTTP(&libReply{header: map[string][]string{}}, req)
	if slot.sess == nil {
		return theauth.ULID{}, theauth.ULID{}, false
	}
	return slot.sess.ID, slot.sess.UserID, true
}

// RevokeAll ends every session of a platform user.
func (s *Sessions) RevokeAll(ctx context.Context, legacyUserID string) error {
	engine, err := s.engineID(ctx, legacyUserID)
	if err != nil || engine == "" {
		return err
	}
	id, err := parseULID(engine)
	if err != nil {
		return fmt.Errorf("authengine: parse engine user id: %w", err)
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	if err := a.RevokeUserSessions(ctx, id); err != nil {
		return fmt.Errorf("authengine: revoke user sessions: %w", err)
	}
	return nil
}

// RevokeAllExcept ends every session of a platform user but keepToken's.
func (s *Sessions) RevokeAllExcept(ctx context.Context, legacyUserID, keepToken string) error {
	keep, _, ok := s.lookupSession(ctx, keepToken)
	if !ok {
		return s.RevokeAll(ctx, legacyUserID)
	}
	engine, err := s.engineID(ctx, legacyUserID)
	if err != nil || engine == "" {
		return err
	}
	id, err := parseULID(engine)
	if err != nil {
		return fmt.Errorf("authengine: parse engine user id: %w", err)
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	if _, err := a.RevokeOtherSessions(ctx, id, keep); err != nil {
		return fmt.Errorf("authengine: revoke other sessions: %w", err)
	}
	return nil
}

// OtherSessionCount counts a platform user's live, fully signed-in sessions
// other than the one exceptToken names (which may be empty).
func (s *Sessions) OtherSessionCount(ctx context.Context, legacyUserID, exceptToken string) (int, error) {
	engine, err := s.engineID(ctx, legacyUserID)
	if err != nil {
		return 0, err
	}
	if engine == "" {
		return 0, nil
	}
	id, err := parseULID(engine)
	if err != nil {
		return 0, fmt.Errorf("authengine: parse engine user id: %w", err)
	}
	var current theauth.ULID
	if exceptToken != "" {
		current, _, _ = s.lookupSession(ctx, exceptToken)
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	list, err := a.ListSessions(ctx, id, current)
	if err != nil {
		return 0, fmt.Errorf("authengine: list sessions: %w", err)
	}
	n := 0
	for _, si := range list {
		if !si.Current {
			n++
		}
	}
	return n, nil
}

// Watch cancels the returned context once token stops validating. Call the
// cancel func when the stream ends.
func (s *Sessions) Watch(ctx context.Context, token string, interval time.Duration) (context.Context, context.CancelCauseFunc) {
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	return a.WatchSession(ctx, token, interval)
}

// MintLink mints a single-use session link for a platform user. The link is
// bound to the user so deleting the user kills it and any session it made.
func (s *Sessions) MintLink(ctx context.Context, u LegacyUser) (string, error) {
	engine, err := s.SyncUser(ctx, u)
	if err != nil {
		return "", err
	}
	id, err := parseULID(engine)
	if err != nil {
		return "", fmt.Errorf("authengine: parse engine user id: %w", err)
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	tok, _, err := a.MintSessionLink(ctx, theauth.MintSessionLinkInput{UserID: id, CredentialID: credentialUserTag + engine})
	if err != nil {
		return "", fmt.Errorf("authengine: mint session link: %w", err)
	}
	return tok, nil
}

// ConsumeLink exchanges a link for a session. ok is false for any invalid,
// expired, used or revoked link.
func (s *Sessions) ConsumeLink(ctx context.Context, link, userAgent, ip string) (token, legacyUserID string, ok bool, err error) {
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	tok, sess, err := a.ConsumeSessionLink(ctx, link, userAgent, hostOnly(ip))
	if errors.Is(err, theauth.ErrSessionLinkInvalid) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("authengine: consume session link: %w", err)
	}
	legacy, err := s.legacyID(ctx, sess.UserID.String())
	if err != nil {
		return "", "", false, err
	}
	if legacy == "" {
		s.Revoke(ctx, tok)
		return "", "", false, nil
	}
	return tok, legacy, true, nil
}

// RequestReset asks the library to mint a reset token for email and mail it
// through the host relay. Unknown emails are a silent no-op.
func (s *Sessions) RequestReset(ctx context.Context, email, ip string) error {
	if err := s.ensureSyncedByEmail(ctx, email); err != nil {
		return err
	}
	_, err := s.call(ctx, routeForgot, map[string]string{"email": email}, callOpts{ip: ip})
	return err
}

// ResetPassword redeems a reset token through the library and returns the
// platform user it belonged to, so the host can mirror the hash for rollback.
func (s *Sessions) ResetPassword(ctx context.Context, token, newPassword, ip string) (legacyUserID string, err error) {
	owner := s.resetTokenOwner(ctx, token)
	if _, err := s.call(ctx, routeReset, map[string]string{"token": token, "newPassword": newPassword}, callOpts{ip: ip}); err != nil {
		return "", err
	}
	return owner, nil
}

func (s *Sessions) resetTokenOwner(ctx context.Context, token string) string {
	var legacy string
	err := s.db.QueryRowContext(ctx, `
		SELECT m.legacy_id FROM theauth_password_reset_tokens t
		JOIN authengine_user_map m ON m.engine_id = t.user_id
		WHERE t.token_hash = ?`, crypto.HashToken(token)).Scan(&legacy)
	if err != nil {
		return ""
	}
	return legacy
}

// MailRelay forwards the library's outgoing mail to a host callback.
type MailRelay struct {
	send func(ctx context.Context, to, subject, body string) error
}

// SetSender installs the host mailer; until then library mail is dropped.
func (m *MailRelay) SetSender(fn func(ctx context.Context, to, subject, body string) error) {
	m.send = fn
}

// Send implements the library's email sender.
func (m *MailRelay) Send(ctx context.Context, to, subject, body string) error {
	if m.send == nil {
		return nil
	}
	return m.send(ctx, to, subject, body)
}

// ResetTokenFromMail extracts the raw reset token from a library reset email
// body, reporting false for any other mail.
func ResetTokenFromMail(body string) (string, bool) {
	_, rest, ok := strings.Cut(body, resetLinkMarker)
	if !ok {
		return "", false
	}
	tok := rest
	if i := strings.IndexAny(rest, "\r\n \t"); i >= 0 {
		tok = rest[:i]
	}
	return tok, tok != ""
}
