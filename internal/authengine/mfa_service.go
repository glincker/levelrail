package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
	theauth "github.com/glincker/theauth-go/v2"
	theauthcrypto "github.com/glincker/theauth-go/v2/crypto"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/oklog/ulid/v2"

	"github.com/GLINCKER/levelrail/internal/totp"
)

var (
	// ErrNotMapped means the Levelrail user has no library counterpart yet (run the backfill).
	ErrNotMapped = errors.New("authengine: user is not mapped to the auth engine")
	// ErrTOTPNotConfigured means the library has no encryption key, so TOTP is off.
	ErrTOTPNotConfigured = errors.New("authengine: totp is not configured")
	// ErrPasskeysNotConfigured means no relying party could be derived.
	ErrPasskeysNotConfigured = errors.New("authengine: passkeys are not configured")
	// ErrNoEnrollment means TOTP confirm was called without a prior begin.
	ErrNoEnrollment = errors.New("authengine: no pending totp enrollment")
	// ErrNotEnrolled means the user has no confirmed TOTP secret.
	ErrNotEnrolled = errors.New("authengine: totp is not enrolled")
	// ErrAlreadyEnrolled means the user already has a confirmed TOTP secret.
	ErrAlreadyEnrolled = errors.New("authengine: totp is already enrolled")
	// ErrInvalidCode means a TOTP or recovery code did not verify.
	ErrInvalidCode = errors.New("authengine: invalid code")
	// ErrPasskeyNotFound means the credential does not exist for this user.
	ErrPasskeyNotFound = errors.New("authengine: passkey not found")
	// ErrPasskeyRejected means a registration or assertion failed verification.
	ErrPasskeyRejected = errors.New("authengine: passkey verification failed")
	// ErrBadPasskeyName means a rename was empty or too long.
	ErrBadPasskeyName = errors.New("authengine: invalid passkey name")
)

// LockedError reports an MFA lockout and how long it lasts.
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string {
	return fmt.Sprintf("authengine: too many failed attempts, retry in %s", e.RetryAfter.Round(time.Second))
}

const enrollmentTTL = 10 * time.Minute

// TOTPEnrollment is what the enrollment begin step hands back to the user.
type TOTPEnrollment struct {
	Secret          string
	ProvisioningURI string
}

// MFAStatus is a user's second-factor state.
type MFAStatus struct {
	Enabled                bool
	RecoveryCodesRemaining int
	// NeedsRegeneration is true for an enrolled user with no usable recovery code.
	NeedsRegeneration bool
}

// Passkey is one registered credential, without key material.
type Passkey struct {
	ID         string
	Label      string
	Transports []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

type pendingEnrollment struct {
	id      string
	expires time.Time
}

// MFA serves TOTP and passkeys through the library, addressed by Levelrail user ids.
type MFA struct {
	auth     *theauth.TheAuth
	store    *sqlitestore.Store
	db       *sql.DB
	totp     bool
	passkeys bool

	mu          sync.Mutex
	enrollments map[string]pendingEnrollment
}

// NewMFA builds the MFA service over the engine's instance and database.
func NewMFA(e *Engine, db *sql.DB) (*MFA, error) {
	if e == nil || db == nil {
		return nil, errors.New("authengine: mfa needs an engine and a database")
	}
	st, err := sqlitestore.New(db)
	if err != nil {
		return nil, fmt.Errorf("authengine: mfa storage: %w", err)
	}
	return &MFA{auth: e.auth, store: st, db: db, totp: e.totp, passkeys: e.passkeys, enrollments: make(map[string]pendingEnrollment)}, nil
}

// TOTPAvailable reports whether the library holds an encryption key for TOTP secrets.
func (m *MFA) TOTPAvailable() bool { return m.totp }

// PasskeysAvailable reports whether a relying party could be derived.
func (m *MFA) PasskeysAvailable() bool { return m.passkeys }

// EngineID maps a Levelrail user id to the library's.
func (m *MFA) EngineID(ctx context.Context, legacyID string) (theauth.ULID, error) {
	var raw string
	err := m.db.QueryRowContext(ctx, `SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, legacyID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return theauth.ULID{}, ErrNotMapped
	}
	if err != nil {
		return theauth.ULID{}, fmt.Errorf("authengine: map user %s: %w", legacyID, err)
	}
	id, err := ulid.Parse(raw)
	if err != nil {
		return theauth.ULID{}, fmt.Errorf("authengine: parse engine id for user %s: %w", legacyID, err)
	}
	return id, nil
}

// LegacyID maps a library user id back to the Levelrail one.
func (m *MFA) LegacyID(ctx context.Context, id theauth.ULID) (string, error) {
	var legacy string
	err := m.db.QueryRowContext(ctx, `SELECT legacy_id FROM authengine_user_map WHERE engine_id = ?`, id.String()).Scan(&legacy)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotMapped
	}
	if err != nil {
		return "", fmt.Errorf("authengine: map engine user %s: %w", id, err)
	}
	return legacy, nil
}

// TOTPBegin starts enrollment and remembers the enrollment id for TOTPFinish.
func (m *MFA) TOTPBegin(ctx context.Context, legacyID, account string) (TOTPEnrollment, error) {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	res, err := m.auth.BeginTOTPEnrollment(ctx, uid, account)
	if err != nil {
		return TOTPEnrollment{}, fmt.Errorf("authengine: begin totp enrollment: %w", err)
	}
	m.mu.Lock()
	m.enrollments[legacyID] = pendingEnrollment{id: res.EnrollmentID, expires: time.Now().Add(enrollmentTTL)}
	m.mu.Unlock()
	return TOTPEnrollment{Secret: res.Secret, ProvisioningURI: res.OTPAuthURL}, nil
}

// TOTPFinish confirms enrollment with a live code and returns the new recovery codes once.
func (m *MFA) TOTPFinish(ctx context.Context, legacyID, code string) ([]string, error) {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	p, ok := m.enrollments[legacyID]
	m.mu.Unlock()
	if !ok || time.Now().After(p.expires) {
		return nil, ErrNoEnrollment
	}
	codes, err := m.auth.FinishTOTPEnrollment(ctx, uid, p.id, code)
	switch {
	case errors.Is(err, theauth.ErrAlreadyEnrolled):
		return nil, ErrAlreadyEnrolled
	case isCode(err, theauth.CodeInvalidTOTP):
		return nil, ErrInvalidCode
	case err != nil:
		return nil, fmt.Errorf("authengine: finish totp enrollment: %w", err)
	}
	m.mu.Lock()
	delete(m.enrollments, legacyID)
	m.mu.Unlock()
	return m.reissueRecoveryCodes(ctx, uid, len(codes))
}

// TOTPStatus reports enrollment and remaining recovery codes.
func (m *MFA) TOTPStatus(ctx context.Context, legacyID string) (MFAStatus, error) {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return MFAStatus{}, err
	}
	st, err := m.auth.TOTPStatus(ctx, uid)
	if err != nil {
		return MFAStatus{}, fmt.Errorf("authengine: totp status: %w", err)
	}
	out := MFAStatus{Enabled: st.Enrolled, RecoveryCodesRemaining: st.RecoveryCodesRemaining}
	if out.RecoveryCodesRemaining < 0 {
		out.RecoveryCodesRemaining = 0
	} else if st.Enrolled && st.RecoveryCodesRemaining == 0 {
		out.NeedsRegeneration = true
	}
	return out, nil
}

// CheckSecondFactor verifies exactly one of code or recoveryCode for the user. The
// library only checks codes against a pending-2FA session, so a throwaway one is minted
// and revoked; replay protection and the per-user lockout still apply.
func (m *MFA) CheckSecondFactor(ctx context.Context, legacyID, code, recoveryCode, userAgent, ip string) error {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return err
	}
	token, sess, err := m.auth.IssuePending2FA(ctx, uid, userAgent, ip)
	if err != nil {
		return fmt.Errorf("authengine: open verification session: %w", err)
	}
	defer func() {
		if rerr := m.auth.RevokeSession(context.WithoutCancel(ctx), sess.ID); rerr != nil {
			slog.Warn("authengine: revoke verification session failed", slog.String("user_id", legacyID), slog.String("error", rerr.Error()))
		}
	}()
	if code != "" {
		_, _, err = m.auth.VerifyTOTP(ctx, token, code)
	} else {
		_, _, err = m.auth.ConsumeRecoveryCode(ctx, token, canonicalRecoveryCode(recoveryCode))
	}
	return mapCodeError(err)
}

func mapCodeError(err error) error {
	if err == nil {
		return nil
	}
	var te *theauth.TheAuthError
	if errors.As(err, &te) {
		switch te.Code {
		case theauth.CodeAccountLocked:
			return &LockedError{RetryAfter: te.RetryAfter}
		case theauth.CodeInvalidTOTP, theauth.CodeInvalidCredentials:
			return ErrInvalidCode
		}
	}
	return fmt.Errorf("authengine: verify second factor: %w", err)
}

func isCode(err error, code string) bool {
	var te *theauth.TheAuthError
	return errors.As(err, &te) && te.Code == code
}

// TOTPDisable removes the user's secret and recovery codes.
func (m *MFA) TOTPDisable(ctx context.Context, legacyID string) error {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return err
	}
	if err := m.store.DeleteTOTPSecret(ctx, uid); err != nil {
		return fmt.Errorf("authengine: delete totp secret: %w", err)
	}
	m.auth.EmitAudit(ctx, "totp.disabled", theauth.TargetRef{Type: "user", ID: uid.String()}, nil)
	return nil
}

// RegenerateRecoveryCodes replaces the user's codes. The caller must have verified a live code.
func (m *MFA) RegenerateRecoveryCodes(ctx context.Context, legacyID string) ([]string, error) {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return nil, err
	}
	codes, err := m.auth.RegenerateRecoveryCodes(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("authengine: regenerate recovery codes: %w", err)
	}
	return m.reissueRecoveryCodes(ctx, uid, len(codes))
}

var libraryRecoveryCode = regexp.MustCompile(`^[0-9a-f]{10}$`)

// canonicalRecoveryCode maps typed input to the form stored in the library:
// codes issued before the format change are bare lowercase hex, the rest are hyphen-free uppercase.
func canonicalRecoveryCode(code string) string {
	if c := strings.ToLower(strings.TrimSpace(code)); libraryRecoveryCode.MatchString(c) {
		return c
	}
	return totp.NormalizeRecoveryCode(code)
}

// reissueRecoveryCodes swaps the library's hex codes for ones in the platform's
// XXXX-XXXX-XXXX-XXXX format, so the API shape does not depend on the engine.
func (m *MFA) reissueRecoveryCodes(ctx context.Context, uid theauth.ULID, count int) ([]string, error) {
	shown := make([]string, 0, count)
	stored := make([]theauth.RecoveryCode, 0, count)
	now := time.Now()
	for range count {
		code, err := totp.GenerateRecoveryCode()
		if err != nil {
			return nil, fmt.Errorf("authengine: generate recovery code: %w", err)
		}
		hash, err := theauthcrypto.HashRecoveryCode(totp.NormalizeRecoveryCode(code))
		if err != nil {
			return nil, fmt.Errorf("authengine: hash recovery code: %w", err)
		}
		shown = append(shown, code)
		stored = append(stored, theauth.RecoveryCode{ID: ulid.Make(), UserID: uid, CodeHash: hash, CreatedAt: now})
	}
	if err := m.store.ReplaceRecoveryCodes(ctx, uid, stored); err != nil {
		return nil, fmt.Errorf("authengine: store recovery codes: %w", err)
	}
	return shown, nil
}

// PasskeyBeginRegistration starts a registration ceremony and returns the options and challenge id.
func (m *MFA) PasskeyBeginRegistration(ctx context.Context, legacyID string) (*protocol.CredentialCreation, string, error) {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return nil, "", err
	}
	creation, tok, err := m.auth.BeginPasskeyRegistration(ctx, uid)
	if err != nil {
		return nil, "", fmt.Errorf("authengine: begin passkey registration: %w", err)
	}
	return creation, tok, nil
}

// PasskeyFinishRegistration verifies the browser's attestation and stores the credential.
func (m *MFA) PasskeyFinishRegistration(ctx context.Context, legacyID, challenge, label string, body io.Reader) (Passkey, error) {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return Passkey{}, err
	}
	cred, err := m.auth.FinishPasskeyRegistration(ctx, uid, challenge, label, body)
	if isCode(err, theauth.CodeWebAuthn) {
		return Passkey{}, ErrPasskeyRejected
	}
	if err != nil {
		return Passkey{}, fmt.Errorf("authengine: finish passkey registration: %w", err)
	}
	return toPasskey(cred), nil
}

// PasskeyBeginLogin starts a discoverable sign-in ceremony.
func (m *MFA) PasskeyBeginLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	assertion, tok, err := m.auth.BeginPasskeyLogin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("authengine: begin passkey login: %w", err)
	}
	return assertion, tok, nil
}

// PasskeyFinishLogin verifies the assertion and returns the Levelrail user id it belongs to.
// The library session it opens is revoked: the caller establishes the session it wants.
func (m *MFA) PasskeyFinishLogin(ctx context.Context, challenge string, body io.Reader, userAgent, ip string) (string, error) {
	_, sess, err := m.auth.FinishPasskeyLogin(ctx, challenge, body, userAgent, ip)
	if err != nil {
		if isCode(err, theauth.CodeWebAuthn) || errors.Is(err, theauth.ErrReplayDetected) {
			return "", ErrPasskeyRejected
		}
		return "", fmt.Errorf("authengine: finish passkey login: %w", err)
	}
	if rerr := m.auth.RevokeSession(context.WithoutCancel(ctx), sess.ID); rerr != nil {
		slog.Warn("authengine: revoke passkey login session failed", slog.String("error", rerr.Error()))
	}
	return m.LegacyID(ctx, sess.UserID)
}

// PasskeyList returns the user's credentials.
func (m *MFA) PasskeyList(ctx context.Context, legacyID string) ([]Passkey, error) {
	uid, err := m.EngineID(ctx, legacyID)
	if err != nil {
		return nil, err
	}
	creds, err := m.store.WebAuthnCredentialsByUserID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("authengine: list passkeys: %w", err)
	}
	out := make([]Passkey, 0, len(creds))
	for _, c := range creds {
		out = append(out, toPasskey(c))
	}
	return out, nil
}

// PasskeyRename sets a credential's label.
func (m *MFA) PasskeyRename(ctx context.Context, legacyID, id, name string) error {
	uid, cid, err := m.passkeyIDs(ctx, legacyID, id)
	if err != nil {
		return err
	}
	err = m.auth.RenamePasskey(ctx, cid, uid, name)
	switch {
	case errors.Is(err, theauth.ErrStorageNotFound):
		return ErrPasskeyNotFound
	case isCode(err, theauth.CodeWebAuthn):
		return ErrBadPasskeyName
	case err != nil:
		return fmt.Errorf("authengine: rename passkey: %w", err)
	}
	return nil
}

// PasskeyDelete removes a credential owned by the user.
func (m *MFA) PasskeyDelete(ctx context.Context, legacyID, id string) error {
	uid, cid, err := m.passkeyIDs(ctx, legacyID, id)
	if err != nil {
		return err
	}
	err = m.store.DeleteWebAuthnCredential(ctx, cid, uid)
	if errors.Is(err, theauth.ErrStorageNotFound) {
		return ErrPasskeyNotFound
	}
	if err != nil {
		return fmt.Errorf("authengine: delete passkey: %w", err)
	}
	m.auth.EmitAudit(ctx, "passkey.deleted", theauth.TargetRef{Type: "webauthn_credential", ID: id}, map[string]any{"user_id": uid.String()})
	return nil
}

func (m *MFA) passkeyIDs(ctx context.Context, legacyID, id string) (uid, cid theauth.ULID, err error) {
	if uid, err = m.EngineID(ctx, legacyID); err != nil {
		return uid, cid, err
	}
	cid, perr := ulid.Parse(id)
	if perr != nil {
		return uid, cid, ErrPasskeyNotFound
	}
	return uid, cid, nil
}

func toPasskey(c theauth.WebAuthnCredential) Passkey {
	return Passkey{ID: c.ID.String(), Label: c.Name, Transports: c.Transports, CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt}
}
