package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envCLIPairingTTL       = "APP_CLI_PAIRING_TTL"
	envCLIPairPerMinute    = "APP_CLI_PAIR_PER_MINUTE"
	envCLIPairCreatePerMin = "APP_CLI_PAIRING_CREATE_PER_MINUTE"
	defaultCLIPairingTTL   = 5 * time.Minute
	maxCLIPairingTTL       = 15 * time.Minute
	cliPairingMaxTries     = 5
	cliPairingGroupLen     = 4
	cliPairingCodeLen      = 2 * cliPairingGroupLen
	cliPairingCreateTries  = 5
	cliTokenLabel          = "cli-pairing"
	errCodeCLIPairing      = "pairing_invalid"
	cliPairingGenericError = "this pairing code is invalid or has expired; copy a new command from the dashboard"
)

type cliPairState struct {
	create     *apiRateLimiter
	redeemIP   *apiRateLimiter
	redeemCode *apiRateLimiter
}

func newCLIPairState() *cliPairState {
	return &cliPairState{
		create:     newAPIRateLimiter(envInt(envCLIPairCreatePerMin, 5)),
		redeemIP:   newAPIRateLimiter(envInt(envCLIPairPerMinute, 10)),
		redeemCode: newAPIRateLimiter(cliPairingMaxTries),
	}
}

type cliPairingStore interface {
	CreateMobilePairingCode(ctx context.Context, c store.MobilePairingCode) error
	GetMobilePairingCodeByLookup(ctx context.Context, lookup string, now time.Time, maxTries int) (store.MobilePairingCode, error)
	RecordMobilePairingFailure(ctx context.Context, id string) error
	ConsumeMobilePairingCode(ctx context.Context, id string, now time.Time) error
	RevokeCLIPairingCodes(ctx context.Context, userID string, now time.Time) (int, error)
}

func (rt *Router) cliPairingStore(w http.ResponseWriter) (cliPairingStore, bool) {
	s, ok := rt.apps.(cliPairingStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "CLI pairing storage is not configured")
	}
	return s, ok
}

func cliPairingTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv(envCLIPairingTTL))
	if raw == "" {
		return defaultCLIPairingTTL
	}
	d, err := parseWindowDuration(raw)
	if err != nil || d <= 0 {
		return defaultCLIPairingTTL
	}
	return min(d, maxCLIPairingTTL)
}

// newCLIPairingCode returns a code from the base32 alphabet, eight characters.
func newCLIPairingCode() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return mobileCodeEncoding.EncodeToString(buf), nil
}

// formatCLIPairingCode groups a code as XXXX-XXXX for display.
func formatCLIPairingCode(code string) string {
	return code[:cliPairingGroupLen] + "-" + code[cliPairingGroupLen:]
}

// normalizeCLIPairingCode strips spaces and dashes and upper-cases, so what a
// person types or pastes compares with what was issued.
func normalizeCLIPairingCode(raw string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, raw))
}

func cliPairingLookup(code string) string {
	return store.CLIPairingLookupPrefix + code[:cliPairingGroupLen]
}

type cliPairingCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	ServerURL string    `json:"server_url"`
}

// handleCreateCLIPairing serves POST /api/v1/auth/cli-pairing: a signed-in
// browser session asks for a code to paste into a terminal. It never answers a
// bearer token, so a leaked token cannot mint its own successor.
func (rt *Router) handleCreateCLIPairing(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.cliPairingStore(w)
	if !ok {
		return
	}
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeError(w, http.StatusForbidden, "a session link cannot pair the CLI")
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return
	}
	if allowed, retry := rt.cliPair.create.allow("user:" + userID); !allowed {
		writeRateLimited(w, retry)
		return
	}
	ctx := r.Context()
	now := time.Now()
	actor := signInActor{kind: auditActorSession, id: userID, name: rt.auditActorName(ctx, auditActorSession, userID, "")}
	if n, err := ps.RevokeCLIPairingCodes(ctx, userID, now); err != nil {
		rt.internalError(w, "api: cli pairing: retire earlier codes failed", err, slog.String("user_id", userID))
		return
	} else if n > 0 {
		rt.auditSignIn(ctx, r, actor, store.AuditActionCLIPairRevoked, cliPairingAuditPath("superseded#"+strconv.Itoa(n)), http.StatusOK)
	}
	for range cliPairingCreateTries {
		code, err := newCLIPairingCode()
		if err != nil {
			rt.internalError(w, "api: cli pairing: generate code failed", err)
			return
		}
		id, err := store.NewMobileID("cpc_")
		if err != nil {
			rt.internalError(w, "api: cli pairing: generate id failed", err)
			return
		}
		rec := store.MobilePairingCode{ID: id, Lookup: cliPairingLookup(code), CodeHash: hashPairingCode(code), OwnerUserID: userID, CreatedAt: now, ExpiresAt: now.Add(cliPairingTTL())}
		if err := ps.CreateMobilePairingCode(ctx, rec); err != nil {
			// A colliding public prefix is the only expected failure; draw again.
			rt.logger.Debug("api: cli pairing: save code failed, retrying", slog.String("code_id", id))
			continue
		}
		rt.auditSignIn(ctx, r, actor, store.AuditActionCLIPairCreated, cliPairingAuditPath(id), http.StatusCreated)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, cliPairingCodeResponse{Code: formatCLIPairingCode(code), ExpiresAt: rec.ExpiresAt, ServerURL: rt.mobileServerURL(r)})
		return
	}
	rt.internalError(w, "api: cli pairing: could not store a code", errors.New("no free lookup"), slog.String("user_id", userID))
}

type cliPairRequest struct {
	Code       string `json:"code"`
	ClientName string `json:"client_name"`
}

type cliPairResponse struct {
	Token     string    `json:"token"`
	TokenID   string    `json:"token_id"`
	Username  string    `json:"username"`
	Abilities []string  `json:"abilities"`
	ExpiresAt time.Time `json:"expires_at"`
}

func cliPairingAuditPath(detail string) string { return "cli-pairing/" + detail }

func (rt *Router) refuseCLIPairing(w http.ResponseWriter, r *http.Request, detail string) {
	rt.auditSignIn(r.Context(), r, anonymousSignIn(""), store.AuditActionCLIPairFailed, cliPairingAuditPath(detail), http.StatusBadRequest)
	writeCodedError(w, http.StatusBadRequest, errCodeCLIPairing, cliPairingGenericError)
}

// handleRedeemCLIPairing serves POST /api/v1/auth/cli-pairing/redeem,
// unauthenticated: the single-use code is the credential. Every refusal looks
// the same to the caller.
func (rt *Router) handleRedeemCLIPairing(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.cliPairingStore(w)
	if !ok {
		return
	}
	var req cliPairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	code := normalizeCLIPairingCode(req.Code)
	lookupKey := code
	if len(lookupKey) > cliPairingGroupLen {
		lookupKey = lookupKey[:cliPairingGroupLen]
	}
	if allowed, retry := allowAll(limiterCheck{rt.cliPair.redeemIP, "ip:" + clientIP(r)}, limiterCheck{rt.cliPair.redeemCode, "code:" + lookupKey}); !allowed {
		writeRateLimited(w, retry)
		return
	}
	if len(code) != cliPairingCodeLen {
		rt.refuseCLIPairing(w, r, "malformed")
		return
	}
	ctx := r.Context()
	now := time.Now()
	rec, err := ps.GetMobilePairingCodeByLookup(ctx, cliPairingLookup(code), now, cliPairingMaxTries)
	if errors.Is(err, store.ErrMobilePairingNotFound) {
		rt.refuseCLIPairing(w, r, "unknown")
		return
	}
	if err != nil {
		rt.internalError(w, "api: cli pairing: lookup failed", err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(rec.CodeHash), []byte(hashPairingCode(code))) != 1 {
		if ferr := ps.RecordMobilePairingFailure(ctx, rec.ID); ferr != nil {
			rt.logger.Warn("api: cli pairing: record failure failed", slog.String("code_id", rec.ID), slog.String("error", ferr.Error()))
		}
		rt.refuseCLIPairing(w, r, rec.ID)
		return
	}
	if err := ps.ConsumeMobilePairingCode(ctx, rec.ID, now); err != nil {
		rt.refuseCLIPairing(w, r, rec.ID)
		return
	}
	user, err := rt.auth.GetUserByID(ctx, rec.OwnerUserID)
	if err != nil {
		rt.refuseCLIPairing(w, r, rec.ID)
		return
	}
	abilities := capDeviceAbilities(user.Abilities)
	ttl := deviceTokenTTL()
	if p, _, perr := rt.effectiveSecurityPolicy(ctx); perr == nil && p.MaxTokenLifetimeDays > 0 {
		ttl = min(ttl, time.Duration(p.MaxTokenLifetimeDays)*24*time.Hour)
	}
	expires := now.Add(ttl)
	label := cliTokenLabel
	if name, ok := cleanDeviceName(req.ClientName); ok {
		label += " " + name
	}
	raw, ids, err := rt.mintOwnedToken(ctx, user.ID, label, abilities, expires)
	if err != nil {
		rt.internalError(w, "api: cli pairing: mint token failed", err, slog.String("user_id", user.ID), slog.String("code_id", rec.ID))
		return
	}
	rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionCLIPaired, cliPairingAuditPath(rec.ID+"#"+ids.legacyID), http.StatusCreated)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, cliPairResponse{Token: raw, TokenID: ids.legacyID, Username: user.Email, Abilities: abilities, ExpiresAt: expires})
}

// revokeCLIPairing closes userID's unused pairing codes after a credential or
// session reset. Failures are logged: the reset itself stands.
func (rt *Router) revokeCLIPairing(ctx context.Context, r *http.Request, userID, reason string) {
	ps, ok := rt.apps.(cliPairingStore)
	if !ok || userID == "" {
		return
	}
	n, err := ps.RevokeCLIPairingCodes(ctx, userID, time.Now())
	if err != nil {
		rt.logger.Warn("api: revoke cli pairing codes failed", slog.String("user_id", userID), slog.String("reason", reason), slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		rt.auditSignIn(ctx, r, anonymousSignIn(userID), store.AuditActionCLIPairRevoked, cliPairingAuditPath(reason+"#"+strconv.Itoa(n)), http.StatusOK)
	}
}
