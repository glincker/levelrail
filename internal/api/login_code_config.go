package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envLoginCodeTTL          = "APP_LOGIN_CODE_TTL"
	envLoginCodeMaxAttempts  = "APP_LOGIN_CODE_MAX_ATTEMPTS"
	envLoginCodeIPRate       = "APP_LOGIN_CODE_RATE_PER_IP_PER_MINUTE"
	envLoginCodeAccountRate  = "APP_LOGIN_CODE_RATE_PER_ACCOUNT_PER_MINUTE"
	envLoginCodeGlobalRate   = "APP_LOGIN_CODE_RATE_GLOBAL_PER_MINUTE"
	envLoginCodeRedeemRate   = "APP_LOGIN_CODE_REDEEM_RATE_PER_IP_PER_MINUTE"
	envLoginCodeMinResponse  = "APP_LOGIN_CODE_MIN_RESPONSE"
	envCodeLoginAdmins       = "APP_AUTH_CODE_LOGIN_ADMINS"
	envCodeLoginOthers       = "APP_AUTH_CODE_LOGIN_OTHERS"
	envDeviceApprovalTTL     = "APP_AUTH_DEVICE_APPROVAL_TTL"
	envTrustedDeviceTTL      = "APP_AUTH_TRUSTED_DEVICE_TTL"
	envNewDeviceApproval     = "APP_AUTH_NEW_DEVICE_APPROVAL"
	defaultLoginCodeTTL      = 10 * time.Minute
	defaultLoginCodeAttempts = 5
	defaultLoginCodeIPRate   = 5
	defaultLoginCodeAcctRate = 3
	defaultLoginCodeGlobal   = 60
	defaultLoginCodeRedeem   = 10
	defaultLoginCodeMinResp  = 300 * time.Millisecond
	defaultDeviceApprovalTTL = 10 * time.Minute
	defaultTrustedDeviceTTL  = 90 * 24 * time.Hour

	loginCodeLength    = 8
	loginCodeAlphabet  = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	loginCodeCookie    = "login_code_binding"
	loginCodeCookiePth = "/api/v1/auth/login-code"
	approvalCookie     = "login_approval_binding"
	approvalCookiePath = "/api/v1/auth/login-approval"
	trustedCookie      = "trusted_device"
	trustedCookiePath  = "/api/v1/auth"
	loginSweepBatch    = 200

	msgLoginCodeSent    = "If that account can sign in with a code, a code was sent to its open dashboard sessions and email. It expires soon."
	msgLoginCodeInvalid = "invalid or expired code"
)

// LoginCodeStore is the store surface sign in with a code, new-device
// approval and trusted devices need.
type LoginCodeStore interface {
	CreateLoginCodeChallenge(ctx context.Context, c store.LoginCodeChallenge) error
	GetLiveLoginCodeByBrowser(ctx context.Context, browserHash string, now time.Time) (store.LoginCodeChallenge, error)
	ReserveLoginCodeAttempt(ctx context.Context, id string, maxAttempts int) (int, bool, error)
	LockLoginCode(ctx context.Context, id string, now time.Time) (bool, error)
	RedeemLoginCode(ctx context.Context, id string, now time.Time) (bool, error)
	ListLiveLoginCodesForUser(ctx context.Context, userID string, now time.Time) ([]store.LoginCodeChallenge, error)
	ClaimLapsedLoginCodes(ctx context.Context, now time.Time, limit int) ([]store.LoginCodeChallenge, error)
	PruneLoginCodes(ctx context.Context, cutoff time.Time) error
	CreateLoginApproval(ctx context.Context, a store.LoginApproval) error
	GetLoginApproval(ctx context.Context, id string) (store.LoginApproval, error)
	GetLoginApprovalByBrowser(ctx context.Context, browserHash string) (store.LoginApproval, error)
	DecideLoginApproval(ctx context.Context, id, userID string, approve bool, decidedBy string, now time.Time) (bool, error)
	ConsumeLoginApproval(ctx context.Context, id string, now time.Time) (bool, error)
	ListPendingLoginApprovalsForUser(ctx context.Context, userID string, now time.Time) ([]store.LoginApproval, error)
	ClaimLapsedLoginApprovals(ctx context.Context, now time.Time, limit int) ([]store.LoginApproval, error)
	CreateTrustedDevice(ctx context.Context, d store.TrustedDevice) error
	GetTrustedDeviceByHash(ctx context.Context, tokenHash string, now time.Time) (store.TrustedDevice, error)
	TouchTrustedDevice(ctx context.Context, id string, now time.Time) error
	ListTrustedDevices(ctx context.Context, userID string, now time.Time) ([]store.TrustedDevice, error)
	RevokeTrustedDevice(ctx context.Context, userID, id string, now time.Time) (bool, error)
	GetCodeLoginSettings(ctx context.Context) (store.CodeLoginSettings, bool, error)
	SaveCodeLoginSettings(ctx context.Context, s store.CodeLoginSettings) error
}

type pendingPlainCode struct {
	userID  string
	code    string
	expires time.Time
}

// codeLoginState holds the limiters and the only plaintext copy of each live
// code, in memory for delivery until it is redeemed, locked or expires.
type codeLoginState struct {
	ttl         time.Duration
	maxAttempts int
	minResponse time.Duration
	approvalTTL time.Duration
	trustTTL    time.Duration
	byIP        *apiRateLimiter
	byAccount   *apiRateLimiter
	global      *apiRateLimiter
	redeemByIP  *apiRateLimiter

	mu    sync.Mutex
	plain map[string]pendingPlainCode
}

func newCodeLoginState() *codeLoginState {
	return &codeLoginState{
		ttl:         envDuration(envLoginCodeTTL, defaultLoginCodeTTL),
		maxAttempts: envInt(envLoginCodeMaxAttempts, defaultLoginCodeAttempts),
		minResponse: envDuration(envLoginCodeMinResponse, defaultLoginCodeMinResp),
		approvalTTL: envDuration(envDeviceApprovalTTL, defaultDeviceApprovalTTL),
		trustTTL:    envDuration(envTrustedDeviceTTL, defaultTrustedDeviceTTL),
		byIP:        newAPIRateLimiter(envInt(envLoginCodeIPRate, defaultLoginCodeIPRate)),
		byAccount:   newAPIRateLimiter(envInt(envLoginCodeAccountRate, defaultLoginCodeAcctRate)),
		global:      newAPIRateLimiter(envInt(envLoginCodeGlobalRate, defaultLoginCodeGlobal)),
		redeemByIP:  newAPIRateLimiter(envInt(envLoginCodeRedeemRate, defaultLoginCodeRedeem)),
		plain:       make(map[string]pendingPlainCode),
	}
}

func (s *codeLoginState) remember(challengeID string, p pendingPlainCode) {
	s.mu.Lock()
	s.plain[challengeID] = p
	s.mu.Unlock()
}

func (s *codeLoginState) forget(challengeID string) {
	s.mu.Lock()
	delete(s.plain, challengeID)
	s.mu.Unlock()
}

// lookup returns the plaintext only to the challenge's own user.
func (s *codeLoginState) lookup(challengeID, userID string, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plain[challengeID]
	if !ok || p.userID != userID || !now.Before(p.expires) {
		return "", false
	}
	return p.code, true
}

func (s *codeLoginState) purgeExpired(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.plain {
		if !now.Before(p.expires) {
			delete(s.plain, id)
		}
	}
}

// NewDeviceApprovalFromEnv reads APP_AUTH_NEW_DEVICE_APPROVAL: on unless set
// to a false value, the break-glass switch for a locked-out operator.
func NewDeviceApprovalFromEnv() bool {
	return envBoolDefault(envNewDeviceApproval, true)
}

func envBoolDefault(name string, def bool) bool {
	v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return def
	}
	return v
}

// newLoginCode draws loginCodeLength symbols from a 32 symbol alphabet; a
// byte masked to 5 bits is uniform, so there is no modulo bias.
func newLoginCode() (string, error) {
	buf := make([]byte, loginCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate login code: %w", err)
	}
	out := make([]byte, loginCodeLength)
	for i, b := range buf {
		out[i] = loginCodeAlphabet[b&31]
	}
	return string(out), nil
}

// normalizeLoginCode accepts lower case, spaces and dashes, and the letters
// people confuse with digits (O, I, L).
func normalizeLoginCode(in string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(in) {
		switch r {
		case ' ', '-':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func formatLoginCode(code string) string {
	if len(code) != loginCodeLength {
		return code
	}
	return code[:4] + "-" + code[4:]
}

func newSalt() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate salt: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func hashLoginCode(salt, code string) string {
	m := hmac.New(sha256.New, []byte(salt))
	m.Write([]byte(code))
	return hex.EncodeToString(m.Sum(nil))
}

func loginCodeMatches(c store.LoginCodeChallenge, code string) bool {
	got := hashLoginCode(c.Salt, code)
	return subtle.ConstantTimeCompare([]byte(got), []byte(c.CodeHash)) == 1
}

// setBindingCookie writes an httpOnly, SameSite=Strict cookie scoped to path.
// An empty value clears it.
func setBindingCookie(w http.ResponseWriter, r *http.Request, name, path, value string, maxAge time.Duration) {
	c := &http.Cookie{ //nolint:gosec // Secure follows the transport, see requestIsHTTPS
		Name: name, Value: value, Path: path, HttpOnly: true,
		Secure: requestIsHTTPS(r), SameSite: http.SameSiteStrictMode,
	}
	if value == "" {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(maxAge.Seconds())
	}
	http.SetCookie(w, c) // NOSONAR: Secure is set whenever the client connection is HTTPS
}

func cookieHash(r *http.Request, name string) (string, bool) {
	c, err := r.Cookie(name)
	if err != nil || c.Value == "" {
		return "", false
	}
	return hashToken(c.Value), true
}

// codeLoginAllowed applies the auth.code_login setting to one account:
// stored settings win, the APP_AUTH_CODE_LOGIN_* env defaults apply otherwise.
func (rt *Router) codeLoginAllowed(ctx context.Context, user *store.User) (bool, error) {
	s, err := rt.codeLoginSettings(ctx)
	if err != nil {
		return false, err
	}
	if hasAbility(user.Abilities, AbilityRoot) {
		return s.Admins, nil
	}
	return s.Others, nil
}

func (rt *Router) codeLoginSettings(ctx context.Context) (store.CodeLoginSettings, error) {
	s, ok, err := rt.loginCodes.GetCodeLoginSettings(ctx)
	if err != nil {
		return s, fmt.Errorf("api: load code login settings: %w", err)
	}
	if !ok {
		s = store.CodeLoginSettings{Admins: envBoolDefault(envCodeLoginAdmins, false), Others: envBoolDefault(envCodeLoginOthers, true)}
	}
	return s, nil
}
