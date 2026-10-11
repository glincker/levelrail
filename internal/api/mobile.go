package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	mobileRoutePrefix         = "/api/v1/mobile/"
	mobilePairingStepUpApp    = "mobile-pairing"
	envMobileAPI              = "APP_MOBILE_API"
	envMobileTokenTTL         = "APP_MOBILE_TOKEN_TTL" //nolint:gosec // env var name
	envMobilePairPerMinute    = "APP_MOBILE_PAIR_PER_MINUTE"
	defaultMobileTokenTTL     = 30 * 24 * time.Hour
	maxMobileTokenTTL         = 365 * 24 * time.Hour
	mobilePairingTTL          = 10 * time.Minute
	mobilePairingMaxTries     = 5
	mobileLookupLen           = 8
	mobileSecretBytes         = 20
	mobileDeviceNameMax       = 64
	mobileTokenNamePrefix     = "Mobile: "
	errCodeMobilePairing      = "pairing_invalid"
	mobilePairingGenericError = "this pairing code is invalid or has expired; create a new one in Settings, Mobile devices"
)

// mobileTokenAbilities is the whole mobile profile: read plus the deploy tier
// (restart, deploy, rollback). Never root, secrets:reveal or signin:approve.
var mobileTokenAbilities = []string{AbilityRead, AbilityDeploy}

var mobilePlatforms = []string{"ios", "android", "other"}

var mobileCodeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

type mobileState struct {
	pairIP   *apiRateLimiter
	pairCode *apiRateLimiter
}

func newMobileState() *mobileState {
	return &mobileState{pairIP: newAPIRateLimiter(envInt(envMobilePairPerMinute, 10)), pairCode: newAPIRateLimiter(mobilePairingMaxTries)}
}

type mobileStore interface {
	CreateMobilePairingCode(ctx context.Context, c store.MobilePairingCode) error
	GetMobilePairingCodeByLookup(ctx context.Context, lookup string, now time.Time, maxTries int) (store.MobilePairingCode, error)
	RecordMobilePairingFailure(ctx context.Context, id string) error
	ConsumeMobilePairingCode(ctx context.Context, id string, now time.Time) error
	CreateMobileDevice(ctx context.Context, d store.MobileDevice) error
	ListMobileDevices(ctx context.Context, ownerID string) ([]store.MobileDevice, error)
	GetMobileDevice(ctx context.Context, id string) (store.MobileDevice, error)
	RevokeMobileDevice(ctx context.Context, id string, now time.Time) error
}

func (rt *Router) mobileStore(w http.ResponseWriter) (mobileStore, bool) {
	s, ok := rt.apps.(mobileStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "mobile pairing storage is not configured")
	}
	return s, ok
}

// mobileAPIKilledByEnv reports APP_MOBILE_API=off, which wins over Settings.
func mobileAPIKilledByEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envMobileAPI))) {
	case "off", "false", "0", "no", "disabled":
		return true
	}
	return false
}

func mobileTokenTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv(envMobileTokenTTL))
	if raw == "" {
		return defaultMobileTokenTTL
	}
	d, err := parseWindowDuration(raw)
	if err != nil {
		return defaultMobileTokenTTL
	}
	return min(d, maxMobileTokenTTL)
}

func hashPairingCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func newPairingCode() (string, error) {
	buf := make([]byte, 5+mobileSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return mobileCodeEncoding.EncodeToString(buf), nil
}

type createPairingRequest struct {
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

type pairingCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	ServerURL string    `json:"server_url"`
	PairURL   string    `json:"pair_url"`
	QRPayload string    `json:"qr_payload"`
}

// pairingOwner confirms a fresh step-up: the dashboard's grant for a session,
// or the token owner's password sent inline for a CLI token.
func (rt *Router) pairingOwner(w http.ResponseWriter, r *http.Request, proof *stepUpProof) (string, bool) {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpSessionOnly, "a session link cannot pair a device")
		return "", false
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		c, err := r.Cookie(sessionCookieName)
		if err == nil {
			if _, granted := rt.envProtect.grants.check(c.Value, userID, stepUpActionManage, mobilePairingStepUpApp, time.Now()); granted {
				return userID, true
			}
		}
		if proof.empty() {
			writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "confirm it's you to pair a device")
			return "", false
		}
		if err := rt.verifyPasswordProof(r, userID, *proof); err != nil {
			writeStepUpError(w, err)
			return "", false
		}
		return userID, true
	}
	raw, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	rec, err := rt.lookupBearerToken(r.Context(), raw)
	if err != nil || rec.OwnerUserID == "" {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpSessionOnly, "pairing needs a token that belongs to a user")
		return "", false
	}
	if gone, gerr := rt.tokenOwnerGone(r.Context(), rec); gerr != nil || gone {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return "", false
	}
	if proof.empty() {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "send the token owner's password (and authenticator code) as step_up to pair a device")
		return "", false
	}
	if err := rt.verifyPasswordProof(r, rec.OwnerUserID, *proof); err != nil {
		writeStepUpError(w, err)
		return "", false
	}
	return rec.OwnerUserID, true
}

// handleCreateMobilePairing serves POST /api/v1/mobile/pairing-codes.
func (rt *Router) handleCreateMobilePairing(w http.ResponseWriter, r *http.Request) {
	ms, ok := rt.mobileStore(w)
	if !ok {
		return
	}
	var req createPairingRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, stepUpBodyLimit)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	ownerID, ok := rt.pairingOwner(w, r, req.StepUp)
	if !ok {
		return
	}
	code, err := newPairingCode()
	if err != nil {
		rt.internalError(w, "api: mobile pairing: generate code failed", err)
		return
	}
	id, err := store.NewMobileID("mpc_")
	if err != nil {
		rt.internalError(w, "api: mobile pairing: generate id failed", err)
		return
	}
	now := time.Now()
	rec := store.MobilePairingCode{ID: id, Lookup: code[:mobileLookupLen], CodeHash: hashPairingCode(code), OwnerUserID: ownerID, CreatedAt: now, ExpiresAt: now.Add(mobilePairingTTL)}
	if err := ms.CreateMobilePairingCode(r.Context(), rec); err != nil {
		rt.internalError(w, "api: mobile pairing: save failed", err, slog.String("user_id", ownerID))
		return
	}
	rt.recordMobileAudit(r, store.AuditActionMobilePairCode, auditActorSession, ownerID, id, http.StatusCreated)
	serverURL := rt.mobileServerURL(r)
	payload, _ := json.Marshal(map[string]string{"server_url": serverURL, "code": code})
	pair := url.Values{"server": {serverURL}, "code": {code}}
	writeJSON(w, http.StatusCreated, pairingCodeResponse{
		Code: code, ExpiresAt: rec.ExpiresAt, ServerURL: serverURL,
		PairURL: rt.brand.ShortName + "://pair?" + pair.Encode(), QRPayload: string(payload),
	})
}

func (rt *Router) mobileServerURL(r *http.Request) string {
	if base := strings.TrimRight(rt.dashboardURL(), "/"); base != "" {
		return base
	}
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

type mobilePairRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
}

type mobilePairResponse struct {
	Token     string    `json:"token"`
	TokenID   string    `json:"token_id"`
	DeviceID  string    `json:"device_id"`
	Abilities []string  `json:"abilities"`
	ExpiresAt time.Time `json:"expires_at"`
	ServerURL string    `json:"server_url"`
}

func cleanDeviceName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > mobileDeviceNameMax {
		return "", false
	}
	for _, c := range s {
		if !unicode.IsPrint(c) {
			return "", false
		}
	}
	return s, true
}

func (rt *Router) refusePairing(w http.ResponseWriter, r *http.Request, detail string) {
	rt.recordMobileAudit(r, store.AuditActionMobilePairFailed, "anonymous", "", detail, http.StatusBadRequest)
	writeCodedError(w, http.StatusBadRequest, errCodeMobilePairing, mobilePairingGenericError)
}

// handleMobilePair serves POST /api/v1/mobile/pair, unauthenticated: the code is the credential.
func (rt *Router) handleMobilePair(w http.ResponseWriter, r *http.Request) {
	ms, ok := rt.mobileStore(w)
	if !ok {
		return
	}
	var req mobilePairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	code := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(req.Code), "-", ""))
	lookup := code
	if len(lookup) > mobileLookupLen {
		lookup = lookup[:mobileLookupLen]
	}
	if allowed, retry := allowAll(limiterCheck{rt.mobile.pairIP, "ip:" + clientIP(r)}, limiterCheck{rt.mobile.pairCode, "code:" + lookup}); !allowed {
		writeRateLimited(w, retry)
		return
	}
	name, nameOK := cleanDeviceName(req.DeviceName)
	if !nameOK || !slices.Contains(mobilePlatforms, req.Platform) {
		writeError(w, http.StatusBadRequest, "device_name (1 to 64 printable characters) and platform (ios, android or other) are required")
		return
	}
	now := time.Now()
	digest := hashPairingCode(code)
	rec, err := ms.GetMobilePairingCodeByLookup(r.Context(), lookup, now, mobilePairingMaxTries)
	if errors.Is(err, store.ErrMobilePairingNotFound) || len(code) <= mobileLookupLen {
		rt.refusePairing(w, r, "unknown")
		return
	}
	if err != nil {
		rt.internalError(w, "api: mobile pair: lookup failed", err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(rec.CodeHash), []byte(digest)) != 1 {
		if ferr := ms.RecordMobilePairingFailure(r.Context(), rec.ID); ferr != nil {
			rt.logger.Warn("api: mobile pair: record failure failed", slog.String("error", ferr.Error()))
		}
		rt.refusePairing(w, r, rec.ID)
		return
	}
	if err := ms.ConsumeMobilePairingCode(r.Context(), rec.ID, now); err != nil {
		rt.refusePairing(w, r, rec.ID)
		return
	}
	abilities, ok := rt.mobileAbilitiesFor(r.Context(), rec.OwnerUserID)
	if !ok {
		rt.refusePairing(w, r, rec.ID)
		return
	}
	rt.issueMobileToken(w, r, ms, rec.OwnerUserID, name, req.Platform, abilities)
}

// mobileAbilitiesFor caps the mobile profile at what the pairing owner still holds.
func (rt *Router) mobileAbilitiesFor(ctx context.Context, ownerID string) ([]string, bool) {
	user, err := rt.auth.GetUserByID(ctx, ownerID)
	if err != nil {
		return nil, false
	}
	out := []string{}
	for _, a := range mobileTokenAbilities {
		if hasAbility(user.Abilities, a) {
			out = append(out, a)
		}
	}
	return out, slices.Contains(out, AbilityRead)
}

func (rt *Router) issueMobileToken(w http.ResponseWriter, r *http.Request, ms mobileStore, ownerID, name, platform string, abilities []string) {
	ctx := r.Context()
	ttl := mobileTokenTTL()
	if p, _, err := rt.effectiveSecurityPolicy(ctx); err == nil && p.MaxTokenLifetimeDays > 0 {
		ttl = min(ttl, time.Duration(p.MaxTokenLifetimeDays)*24*time.Hour)
	}
	expires := time.Now().Add(ttl)
	legacyID, err := randomTokenID()
	if err != nil {
		rt.internalError(w, "api: mobile pair: token id failed", err)
		return
	}
	raw, lib, err := rt.authLib.tokens.MintToken(ctx, authengine.MintInput{OwnerLegacyID: ownerID, Name: mobileTokenNamePrefix + name, Abilities: abilities, ExpiresAt: &expires})
	if err != nil {
		rt.internalError(w, "api: mobile pair: mint failed", err)
		return
	}
	legacy := store.APIToken{ID: legacyID, Name: lib.Name, TokenHash: lib.HashHex, Abilities: abilities, CreatedAt: lib.CreatedAt, ExpiresAt: &expires, OwnerUserID: ownerID}
	ids := cliConsoleTokenIDs{engineID: lib.EngineID}
	if err := rt.tokens.SaveAPIToken(ctx, legacy); err != nil {
		rt.revokeCLIConsoleToken(ids)
		rt.internalError(w, "api: mobile pair: mirror save failed", err)
		return
	}
	ids.legacyID = legacyID
	if err := rt.authLib.tokens.LinkToken(ctx, legacyID, lib.EngineID); err != nil {
		rt.revokeCLIConsoleToken(ids)
		rt.internalError(w, "api: mobile pair: link failed", err)
		return
	}
	deviceID, err := store.NewMobileID("mdv_")
	if err == nil {
		err = ms.CreateMobileDevice(ctx, store.MobileDevice{ID: deviceID, TokenID: legacyID, OwnerUserID: ownerID, Name: name, Platform: platform, CreatedAt: time.Now()})
	}
	if err != nil {
		rt.revokeCLIConsoleToken(ids)
		rt.internalError(w, "api: mobile pair: save device failed", err)
		return
	}
	rt.recordMobileAudit(r, store.AuditActionMobilePaired, auditActorSession, ownerID, deviceID, http.StatusCreated)
	writeJSON(w, http.StatusCreated, mobilePairResponse{Token: raw, TokenID: legacyID, DeviceID: deviceID, Abilities: abilities, ExpiresAt: expires, ServerURL: rt.mobileServerURL(r)})
}

func (rt *Router) recordMobileAudit(r *http.Request, action, actorType, actorID, subject string, status int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, Ability: AbilityWrite,
		Method: r.Method, Path: r.URL.Path + " (" + subject + ")", StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: action,
	}
	if actorID != "" {
		entry.ActorName = rt.auditActorName(ctx, actorType, actorID, "")
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save mobile audit entry failed", slog.String("error", err.Error()))
	}
}
