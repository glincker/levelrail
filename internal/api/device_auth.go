package api

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"
)

// deviceAuthTTL is how long a device/user code pair stays valid before
// a poll or an approval attempt gets "expired_token": long enough for
// an operator to switch to a browser and type an 8-character code, not
// so long a forgotten terminal window stays exploitable.
const deviceAuthTTL = 10 * time.Minute

// devicePollInterval is the CLI's own minimum wait between polls,
// returned as part of the start response, RFC 8628's own "interval"
// field: a fixed value is enough here, this flow has no need for the
// "slow_down"-response escalation real OAuth device grants define.

const (
	envDeviceCodeTTL        = "APP_DEVICE_CODE_TTL"              //nolint:gosec // env var name
	envDeviceStartRate      = "APP_DEVICE_START_RATE_PER_MINUTE" //nolint:gosec // env var name
	defaultDeviceStartRate  = 6
	envDeviceTokenTTLDays   = "APP_DEVICE_TOKEN_TTL_DAYS"   //nolint:gosec // env var name
	envDeviceTokenAllowRoot = "APP_DEVICE_TOKEN_ALLOW_ROOT" //nolint:gosec // env var name
	defaultDeviceTokenDays  = 30
)

// deviceStartRatePerMinute is the per-IP budget for unauthenticated device
// login starts, env-overridable; 0 or less disables the limit.
func deviceStartRatePerMinute() int {
	if n, err := strconv.Atoi(os.Getenv(envDeviceStartRate)); err == nil {
		return n
	}
	return defaultDeviceStartRate
}

// deviceTokenTTL is how long a device-flow token lives, env-overridable.
func deviceTokenTTL() time.Duration {
	days := defaultDeviceTokenDays
	if n, err := strconv.Atoi(os.Getenv(envDeviceTokenTTLDays)); err == nil && n > 0 {
		days = n
	}
	return time.Duration(days) * 24 * time.Hour
}

// capDeviceAbilities keeps a device token to the approver's abilities,
// but never silently root: a root approver gets every non-root ability
// unless APP_DEVICE_TOKEN_ALLOW_ROOT=true.
func capDeviceAbilities(approver []string) []string {
	if !slices.Contains(approver, AbilityRoot) || os.Getenv(envDeviceTokenAllowRoot) == "true" {
		return approver
	}
	capped := make([]string, 0, len(validAbilities))
	for _, a := range validAbilities {
		if a != AbilityRoot {
			capped = append(capped, a)
		}
	}
	return capped
}

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
	if rt.deviceFlow != nil {
		if ok, retryAfter := rt.deviceFlow.allow(clientIP(r)); !ok {
			writeRateLimited(w, retryAfter)
			return
		}
	}

	var req deviceStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rt.libraryDeviceStart(w, r, req)
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

	rt.libraryDeviceToken(w, r, req.DeviceCode)
}

type deviceAuthRequestResource struct {
	UserCode    string    `json:"user_code"`
	ClientName  string    `json:"client_name"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	RequesterIP string    `json:"requester_ip"`
	UserAgent   string    `json:"user_agent"`
	// IPMismatch is true when the requesting IP differs from the IP this
	// listing session came from, a hint the login may not be the operator's own.
	IPMismatch bool `json:"ip_mismatch"`
}

// handleListDeviceAuthRequests handles GET /api/v1/auth/device/requests:
// the web dashboard's own approval queue. Session-only (requireAuth,
// router.go), the same tier as the token endpoints this flow ultimately
// mints from: any signed-in operator can see and act on a pending
// device login, since approving one only ever grants a token scoped to
// their own abilities.
func (rt *Router) handleListDeviceAuthRequests(w http.ResponseWriter, r *http.Request) {
	rt.libraryListDeviceRequests(w, r)
}

// handleApproveDeviceAuthRequest handles POST
// /api/v1/auth/device/{user_code}/approve.
func (rt *Router) handleApproveDeviceAuthRequest(w http.ResponseWriter, r *http.Request) {
	rt.decideDeviceAuthRequest(w, r, true)
}

// handleDenyDeviceAuthRequest handles POST
// /api/v1/auth/device/{user_code}/deny.
func (rt *Router) handleDenyDeviceAuthRequest(w http.ResponseWriter, r *http.Request) {
	rt.decideDeviceAuthRequest(w, r, false)
}

func (rt *Router) decideDeviceAuthRequest(w http.ResponseWriter, r *http.Request, approve bool) {
	userCode := r.PathValue("user_code")
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	rt.libraryDecideDevice(w, r, userID, userCode, approve)
}
