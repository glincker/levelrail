package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DeviceAuthStore is the store surface the device-login flow needs,
// defined here next to the handlers that use it, the same
// "single-file feature" shape PolicyStore/OnboardingStore already use.
type DeviceAuthStore interface {
	SaveDeviceAuthRequest(ctx context.Context, r store.DeviceAuthRequest) error
	GetDeviceAuthRequestByDeviceCode(ctx context.Context, deviceCode string) (*store.DeviceAuthRequest, error)
	GetDeviceAuthRequestByUserCode(ctx context.Context, userCode string) (*store.DeviceAuthRequest, error)
	ListPendingDeviceAuthRequests(ctx context.Context, now time.Time) ([]store.DeviceAuthRequest, error)
	SetDeviceAuthRequestStatus(ctx context.Context, userCode, status, approvedByUserID string) (int64, error)
	RedeemDeviceAuthRequest(ctx context.Context, deviceCode, tokenID string, redeemedAt time.Time) error
}

// deviceAuthTTL is how long a device/user code pair stays valid before
// a poll or an approval attempt gets "expired_token": long enough for
// an operator to switch to a browser and type an 8-character code, not
// so long a forgotten terminal window stays exploitable.
const deviceAuthTTL = 10 * time.Minute

// devicePollInterval is the CLI's own minimum wait between polls,
// returned as part of the start response, RFC 8628's own "interval"
// field: a fixed value is enough here, this flow has no need for the
// "slow_down"-response escalation real OAuth device grants define.
const devicePollInterval = 5

type deviceStartRequest struct {
	// ClientName is an optional operator-facing label (e.g. a hostname)
	// so the web approval UI can show "log in from macbook-pro" instead
	// of an opaque code; never trusted for anything security-relevant.
	ClientName string `json:"client_name"`
}

type deviceStartResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// handleDeviceAuthStart handles POST /api/v1/auth/device/start: the
// CLI's first call, fully unauthenticated (there is no credential yet).
// Public, but per-IP rate limited (deviceFlow) since it's a free write.
func (rt *Router) handleDeviceAuthStart(w http.ResponseWriter, r *http.Request) {
	ipKey := clientIP(r)
	if rt.deviceFlow != nil {
		if ok, retryAfter := rt.deviceFlow.allow(ipKey); !ok {
			writeRateLimited(w, retryAfter)
			return
		}
		rt.deviceFlow.recordFailure(ipKey)
	}

	var req deviceStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id, err := store.NewDeviceAuthRequestID()
	if err != nil {
		rt.internalError(w, "api: device auth start: generate id failed", err)
		return
	}
	deviceCode, err := store.NewDeviceCode()
	if err != nil {
		rt.internalError(w, "api: device auth start: generate device code failed", err)
		return
	}
	userCode, err := store.NewUserCode()
	if err != nil {
		rt.internalError(w, "api: device auth start: generate user code failed", err)
		return
	}

	now := time.Now()
	rec := store.DeviceAuthRequest{
		ID:         id,
		DeviceCode: deviceCode,
		UserCode:   userCode,
		ClientName: req.ClientName,
		CreatedAt:  now,
		ExpiresAt:  now.Add(deviceAuthTTL),
	}
	if err := rt.deviceAuth.SaveDeviceAuthRequest(r.Context(), rec); err != nil {
		rt.internalError(w, "api: device auth start: save failed", err)
		return
	}

	base := requestBaseURL(r)
	writeJSON(w, http.StatusCreated, deviceStartResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         base + "/settings/cli-access",
		VerificationURIComplete: base + "/settings/cli-access?user_code=" + userCode,
		ExpiresIn:               int(deviceAuthTTL.Seconds()),
		Interval:                devicePollInterval,
	})
}

type deviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

// handleDeviceAuthToken handles POST /api/v1/auth/device/token: the
// CLI's poll, fully unauthenticated like start above (device_code
// itself, 256 random bits, is the only credential this call has or
// needs). Mints a real API token exactly once, the moment an operator's
// approval is first observed, mirroring RFC 8628's own error vocabulary
// (authorization_pending/access_denied/expired_token) so a future
// client could reuse a standard device-flow library against this
// endpoint even though nothing in this codebase does yet.
func (rt *Router) handleDeviceAuthToken(w http.ResponseWriter, r *http.Request) {
	var req deviceTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeviceCode == "" {
		writeError(w, http.StatusBadRequest, "device_code is required")
		return
	}

	rec, err := rt.deviceAuth.GetDeviceAuthRequestByDeviceCode(r.Context(), req.DeviceCode)
	if errors.Is(err, store.ErrDeviceAuthRequestNotFound) {
		writeError(w, http.StatusBadRequest, "expired_token")
		return
	}
	if err != nil {
		rt.internalError(w, "api: device auth token: lookup failed", err)
		return
	}

	if rec.RedeemedAt != nil {
		writeError(w, http.StatusBadRequest, "expired_token")
		return
	}
	if time.Now().After(rec.ExpiresAt) {
		writeError(w, http.StatusBadRequest, "expired_token")
		return
	}

	switch rec.Status {
	case store.DeviceAuthStatusPending:
		writeError(w, http.StatusBadRequest, "authorization_pending")
		return
	case store.DeviceAuthStatusDenied:
		writeError(w, http.StatusBadRequest, "access_denied")
		return
	}

	if rec.ApprovedByUserID == nil {
		rt.internalError(w, "api: device auth token: approved request missing approver", errors.New("approved_by_user_id is nil"))
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), *rec.ApprovedByUserID)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusBadRequest, "access_denied")
		return
	}
	if err != nil {
		rt.internalError(w, "api: device auth token: load approving user failed", err)
		return
	}

	plaintext, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: device auth token: generate token failed", err)
		return
	}
	tokenID, err := randomTokenID()
	if err != nil {
		rt.internalError(w, "api: device auth token: generate token id failed", err)
		return
	}

	tokenName := "cli login"
	if rec.ClientName != "" {
		tokenName = "cli login: " + rec.ClientName
	}
	tokenRec := store.APIToken{
		ID:        tokenID,
		Name:      tokenName,
		TokenHash: hashToken(plaintext),
		// Scoped to exactly what the approving operator can already do:
		// an unauthenticated device request never gets to pick its own
		// abilities, it inherits the approver's, the same "logging in as
		// yourself from a new device" guarantee a session cookie gives.
		Abilities: user.Abilities,
		CreatedAt: time.Now(),
	}
	if err := rt.tokens.SaveAPIToken(r.Context(), tokenRec); err != nil {
		rt.internalError(w, "api: device auth token: save token failed", err)
		return
	}

	if err := rt.deviceAuth.RedeemDeviceAuthRequest(r.Context(), req.DeviceCode, tokenID, time.Now()); err != nil {
		// The token row above is already committed; a redeem race lost
		// here just means a second concurrent poll would also see
		// ErrDeviceAuthRequestNotFound below and get expired_token, never
		// a second token. Not ideal (an orphaned token stays valid and
		// revocable via the normal tokens list), but never a security
		// gap: the code has already done its one job.
		rt.internalError(w, "api: device auth token: redeem failed", err)
		return
	}

	writeJSON(w, http.StatusOK, createTokenResponse{
		tokenResource: toTokenResource(tokenRec),
		Token:         plaintext,
	})
}

type deviceAuthRequestResource struct {
	UserCode   string    `json:"user_code"`
	ClientName string    `json:"client_name"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// handleListDeviceAuthRequests handles GET /api/v1/auth/device/requests:
// the web dashboard's own approval queue. Session-only (requireAuth,
// router.go), the same tier as the token endpoints this flow ultimately
// mints from: any signed-in operator can see and act on a pending
// device login, since approving one only ever grants a token scoped to
// their own abilities.
func (rt *Router) handleListDeviceAuthRequests(w http.ResponseWriter, r *http.Request) {
	recs, err := rt.deviceAuth.ListPendingDeviceAuthRequests(r.Context(), time.Now())
	if err != nil {
		rt.internalError(w, "api: list device auth requests failed", err)
		return
	}
	out := make([]deviceAuthRequestResource, 0, len(recs))
	for _, rec := range recs {
		out = append(out, deviceAuthRequestResource{
			UserCode:   rec.UserCode,
			ClientName: rec.ClientName,
			CreatedAt:  rec.CreatedAt,
			ExpiresAt:  rec.ExpiresAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleApproveDeviceAuthRequest handles POST
// /api/v1/auth/device/{user_code}/approve.
func (rt *Router) handleApproveDeviceAuthRequest(w http.ResponseWriter, r *http.Request) {
	rt.decideDeviceAuthRequest(w, r, store.DeviceAuthStatusApproved)
}

// handleDenyDeviceAuthRequest handles POST
// /api/v1/auth/device/{user_code}/deny.
func (rt *Router) handleDenyDeviceAuthRequest(w http.ResponseWriter, r *http.Request) {
	rt.decideDeviceAuthRequest(w, r, store.DeviceAuthStatusDenied)
}

func (rt *Router) decideDeviceAuthRequest(w http.ResponseWriter, r *http.Request, status string) {
	userCode := r.PathValue("user_code")
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	approver := ""
	if status == store.DeviceAuthStatusApproved {
		approver = userID
	}
	n, err := rt.deviceAuth.SetDeviceAuthRequestStatus(r.Context(), userCode, status, approver)
	if err != nil {
		rt.internalError(w, "api: decide device auth request failed", err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "no pending device login with this code")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requestBaseURL builds this request's own scheme+host, the same
// X-Forwarded-Proto-aware pattern oauthRedirectURL (oauth.go) already
// establishes: correct for whatever domain the operator is actually
// reaching this control plane through, never a fixed setting.
func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
