package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

const (
	stepUpBodyLimit = 16 << 10
	maxStepUpAppLen = 253
)

type stepUpOptionsResponse struct {
	Methods          []string `json:"methods"`
	CodeRequired     bool     `json:"code_required"`
	Policy           string   `json:"policy"`
	PolicyValid      bool     `json:"policy_valid"`
	GrantTTLSeconds  int      `json:"grant_ttl_seconds"`
	HideAfterSeconds int      `json:"hide_after_seconds"`
	TokenReveal      bool     `json:"token_reveal"`
}

// handleStepUpOptions handles GET /api/v1/auth/step-up: which proofs this
// signed-in account can give, plus the reveal policy the dashboard explains.
func (rt *Router) handleStepUpOptions(w http.ResponseWriter, r *http.Request) {
	policy, valid := revealPolicy()
	resp := stepUpOptionsResponse{
		Methods: []string{}, Policy: policy, PolicyValid: valid,
		GrantTTLSeconds:  int(stepUpTTL().Seconds()),
		HideAfterSeconds: int(revealHideAfter().Seconds()),
		TokenReveal:      tokenRevealAllowed(),
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		if rt.userHasPassword(r.Context(), userID) {
			resp.Methods = append(resp.Methods, stepUpMethodPassword)
		}
		if on, err := rt.totpEnabled(r.Context(), userID); err == nil && on {
			resp.CodeRequired = true
		}
		if rt.userHasPasskeys(r.Context(), userID) {
			resp.Methods = append(resp.Methods, stepUpMethodPasskey)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type stepUpRequest struct {
	stepUpProof
	Action string `json:"action"`
	App    string `json:"app"`
}

type stepUpResponse struct {
	Action    string    `json:"action"`
	App       string    `json:"app"`
	Method    string    `json:"method"`
	ExpiresAt time.Time `json:"expires_at"`
}

// validStepUpScope checks the scope's shape only. App existence is not
// checked, so step-up never tells a caller which apps exist; the grant is
// useless without the resource-gated reveal route.
func validStepUpScope(w http.ResponseWriter, action, app string) bool {
	if !validStepUpActions[action] || app == "" || len(app) > maxStepUpAppLen {
		writeError(w, http.StatusBadRequest, "action must be reveal, export or manage, and app is required")
		return false
	}
	return true
}

// handleStepUp handles POST /api/v1/auth/step-up: verifies the password
// (plus the authenticator code when enrolled) and grants one scope.
func (rt *Router) handleStepUp(w http.ResponseWriter, r *http.Request) {
	userID, token, ok := rt.sessionUserOnly(w, r)
	if !ok {
		return
	}
	var req stepUpRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, stepUpBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Method == "" {
		req.Method = stepUpMethodPassword
	}
	if req.Method != stepUpMethodPassword {
		writeError(w, http.StatusBadRequest, "method must be password; passkeys use /api/v1/auth/step-up/passkey/begin")
		return
	}
	if !validStepUpScope(w, req.Action, req.App) {
		return
	}
	if err := rt.verifyPasswordProof(r, userID, req.stepUpProof); err != nil {
		writeStepUpError(w, err)
		return
	}
	exp := rt.envProtect.grants.issue(token, userID, req.Action, req.App, stepUpMethodPassword, stepUpTTL(), time.Now())
	rt.logger.Info("api: step-up granted", slog.String("user_id", userID), slog.String("app", req.App), slog.String("action", req.Action), slog.String("method", stepUpMethodPassword))
	writeJSON(w, http.StatusOK, stepUpResponse{Action: req.Action, App: req.App, Method: stepUpMethodPassword, ExpiresAt: exp})
}

// handleStepUpPasskeyBegin handles POST /api/v1/auth/step-up/passkey/begin.
func (rt *Router) handleStepUpPasskeyBegin(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := rt.sessionUserOnly(w, r)
	if !ok {
		return
	}
	if !rt.userHasPasskeys(r.Context(), userID) {
		writeCodedError(w, http.StatusBadRequest, "step_up_no_passkey", "this account has no passkey registered")
		return
	}
	if err := rt.stepUpAllowed(r, userID); err != nil {
		writeStepUpError(w, err)
		return
	}
	options, challenge, err := rt.mfaLib.mfa.PasskeyBeginLogin(r.Context())
	if err != nil {
		rt.internalError(w, "api: step-up passkey begin failed", err, slog.String("user_id", userID))
		return
	}
	writeJSON(w, http.StatusOK, passkeyLoginBeginResponse{SessionID: challenge, Options: options})
}

// handleStepUpPasskeyFinish handles POST
// /api/v1/auth/step-up/passkey/finish?session_id=&action=&app=. The
// assertion must belong to the signed-in account.
func (rt *Router) handleStepUpPasskeyFinish(w http.ResponseWriter, r *http.Request) {
	userID, token, ok := rt.sessionUserOnly(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	challenge, action, app := q.Get("session_id"), q.Get("action"), q.Get("app")
	if challenge == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	if !validStepUpScope(w, action, app) {
		return
	}
	if rt.mfaLib == nil || rt.mfaLib.mfa == nil {
		writeError(w, http.StatusNotImplemented, msgPasskeysOff)
		return
	}
	if err := rt.stepUpAllowed(r, userID); err != nil {
		writeStepUpError(w, err)
		return
	}
	owner, err := rt.mfaLib.mfa.PasskeyFinishLogin(r.Context(), challenge, http.MaxBytesReader(w, r.Body, passkeyBodyLimit), r.UserAgent(), clientIP(r))
	if err != nil && !errors.Is(err, authengine.ErrPasskeyRejected) && !errors.Is(err, authengine.ErrNotMapped) {
		rt.internalError(w, "api: step-up passkey finish failed", err, slog.String("user_id", userID))
		return
	}
	verified := err == nil && owner == userID
	rt.recordStepUp(r, userID, verified)
	rt.auditStepUp(r.Context(), r, userID, verified)
	if !verified {
		writeStepUpError(w, errStepUpInvalid)
		return
	}
	exp := rt.envProtect.grants.issue(token, userID, action, app, stepUpMethodPasskey, stepUpTTL(), time.Now())
	rt.logger.Info("api: step-up granted", slog.String("user_id", userID), slog.String("app", app), slog.String("action", action), slog.String("method", stepUpMethodPasskey))
	writeJSON(w, http.StatusOK, stepUpResponse{Action: action, App: app, Method: stepUpMethodPasskey, ExpiresAt: exp})
}
