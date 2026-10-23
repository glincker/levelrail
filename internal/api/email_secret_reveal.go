package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	emailRevealStepUpApp     = "email-secret"
	envEmailRevealPerMin     = "APP_EMAIL_REVEAL_PER_MINUTE"
	defaultEmailRevealRate   = 3
	errCodeTwoFactorRequired = "two_factor_required"
)

type emailSecretRevealRequest struct {
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

// emailSecretRevealResponse carries the value and nothing else: no field name, no hint of its length or tail.
type emailSecretRevealResponse struct {
	Value string `json:"value"`
}

func newEmailRevealLimiter() *apiRateLimiter {
	n := defaultEmailRevealRate
	if v, err := strconv.Atoi(os.Getenv(envEmailRevealPerMin)); err == nil && v > 0 {
		n = v
	}
	return newAPIRateLimiter(n)
}

// revealStatusWriter remembers the status a step-up helper wrote, so a refused attempt can be audited.
type revealStatusWriter struct {
	http.ResponseWriter
	status int
}

func (s *revealStatusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *revealStatusWriter) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// handleRevealEmailSecret handles POST /api/v1/settings/email/providers/{id}/secrets/{field}/reveal.
// Root plus a fresh step-up scoped to the provider, and the account must have TOTP enrolled. The value is never
// logged, cached or audited.
func (rt *Router) handleRevealEmailSecret(w http.ResponseWriter, r *http.Request) {
	if rt.emailSecrets == nil {
		writeError(w, http.StatusNotImplemented, "email settings are not configured on this control plane (no master key set)")
		return
	}
	id, field := r.PathValue("id"), r.PathValue("field")
	envKey := ""
	for _, f := range emailSecretFields {
		if f.envKey == field {
			envKey = f.envKey
		}
	}
	if envKey == "" {
		writeError(w, http.StatusNotFound, "unknown credential field")
		return
	}
	var req emailSecretRevealRequest
	if err := decodeJSONBodyLimit(r, &req, stepUpBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	sw := &revealStatusWriter{ResponseWriter: w}
	userID, ok := rt.stepUpOwner(sw, r, req.StepUp, emailRevealStepUpApp+":"+id, "reveal an email credential", "revealing credentials")
	if !ok {
		rt.auditEmailSecretReveal(r, id, field, "", sw.status)
		return
	}
	enrolled, err := rt.totpEnabled(r.Context(), userID)
	if err != nil || !enrolled {
		rt.auditEmailSecretReveal(r, id, field, userID, http.StatusForbidden)
		writeCodedError(w, http.StatusForbidden, errCodeTwoFactorRequired, "revealing an email credential needs two-factor authentication: set it up under account security first")
		return
	}
	if allowed, retry := rt.emailRevealLimit.allow(userID); !allowed {
		rt.auditEmailSecretReveal(r, id, field, userID, http.StatusTooManyRequests)
		writeRateLimited(w, retry)
		return
	}
	p, err := rt.emailProviders.GetEmailProvider(r.Context(), id)
	if err != nil {
		rt.auditEmailSecretReveal(r, id, field, userID, http.StatusNotFound)
		rt.writeProviderError(w, "api: reveal email secret: get provider failed", err)
		return
	}
	value, err := rt.emailSecrets.Resolve(r.Context(), store.EmailProviderSecretsKey(p.Role, p.ID), envKey)
	if errors.Is(err, secrets.ErrValueNotFound) || (err == nil && value == "") {
		rt.auditEmailSecretReveal(r, id, field, userID, http.StatusNotFound)
		writeError(w, http.StatusNotFound, "no credential is stored for this field")
		return
	}
	if err != nil {
		rt.auditEmailSecretReveal(r, id, field, userID, http.StatusInternalServerError)
		rt.internalError(w, "api: reveal email secret failed", err)
		return
	}
	rt.auditEmailSecretReveal(r, id, field, userID, http.StatusOK)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, emailSecretRevealResponse{Value: value})
}

// auditEmailSecretReveal records an attempt with the provider id and field name, never the value or the proof.
func (rt *Router) auditEmailSecretReveal(r *http.Request, providerID, field, userID string, status int) {
	if rt.auditLog == nil {
		return
	}
	if status == 0 {
		status = http.StatusForbidden
	}
	entryID, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: email secret reveal audit id failed", slog.String("provider", providerID))
		return
	}
	detail, err := json.Marshal(map[string]any{"provider_id": providerID, "field": field})
	if err != nil {
		return
	}
	actorType, actorID, actorName, _ := rt.currentActor(r)
	if actorID == "" {
		actorID = userID
	}
	err = rt.auditLog.SaveAuditEntry(r.Context(), store.AuditEntry{
		ID: entryID, ActorType: actorType, ActorID: actorID, ActorName: actorName, Ability: AbilityRoot,
		Method: r.Method, Path: r.URL.Path, StatusCode: status, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()),
		Action: store.AuditActionEmailSecretRevealed, Detail: string(detail),
	})
	if err != nil {
		rt.logger.Warn("api: email secret reveal audit failed", slog.String("provider", providerID), slog.String("error", err.Error()))
	}
}
