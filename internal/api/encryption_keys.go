package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EncryptionKeyRotator is the per-app data key surface of internal/secrets.DEKRotator.
type EncryptionKeyRotator interface {
	RotateOne(ctx context.Context, serviceName string) (secrets.DEKRotateResult, error)
	RotateAll(ctx context.Context, actor string) (secrets.DEKRotateAllResult, error)
	Status(ctx context.Context) (secrets.DEKStatus, error)
}

// SecretKeyPolicyStore persists the maximum key age policy.
type SecretKeyPolicyStore interface {
	GetSecretKeyPolicy(ctx context.Context) (store.SecretKeyPolicy, error)
	SaveSecretKeyPolicy(ctx context.Context, p store.SecretKeyPolicy, now time.Time) error
}

type encryptionKeysDeps struct {
	rotator EncryptionKeyRotator
	policy  SecretKeyPolicyStore
}

// SetEncryptionKeys enables the /api/v1/system/encryption-keys routes.
func (rt *Router) SetEncryptionKeys(r EncryptionKeyRotator, p SecretKeyPolicyStore) {
	rt.encryptionKeys = encryptionKeysDeps{rotator: r, policy: p}
}

const (
	encryptionKeysStepUpApp = "encryption-keys"
	// defaultMasterKeyHintAgeDays is when the master key hint appears with no policy set.
	defaultMasterKeyHintAgeDays = 90
	maxKeyAgePolicyDays         = 3650
	maxRemindEveryDays          = 365

	keyFixRotateMasterKey = "rotate_master_key"
	keyFixRotateApp       = "rotate_dek"
	keyFixRotateAll       = "rotate_all_deks"
	keyFixEnablePolicy    = "enable_key_policy"
)

type encryptionKeyFix struct {
	Kind string `json:"kind"`
	App  string `json:"app,omitempty"`
}

type encryptionKeyHint struct {
	Code     string           `json:"code"`
	Severity string           `json:"severity"`
	Message  string           `json:"message"`
	Fix      encryptionKeyFix `json:"fix"`
}

type appKeyResource struct {
	App        string     `json:"app"`
	CreatedAt  time.Time  `json:"created_at"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	AgeDays    int        `json:"age_days"`
	ValueCount int        `json:"value_count"`
	Overdue    bool       `json:"overdue"`
}

type masterKeyStatusResource struct {
	Configured bool       `json:"configured"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	AgeDays    *int       `json:"age_days,omitempty"`
	Overdue    bool       `json:"overdue"`
}

type keyPolicyResource struct {
	MaxKeyAgeDays   int        `json:"max_key_age_days"`
	RemindEveryDays int        `json:"remind_every_days"`
	LastRemindedAt  *time.Time `json:"last_reminded_at,omitempty"`
}

type rotationRunResource struct {
	ID         string     `json:"id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Failed     int        `json:"failed"`
	Actor      string     `json:"actor,omitempty"`
}

type encryptionKeysResource struct {
	MasterKey masterKeyStatusResource `json:"master_key"`
	Apps      []appKeyResource        `json:"apps"`
	Policy    keyPolicyResource       `json:"policy"`
	LastRun   *rotationRunResource    `json:"last_run,omitempty"`
	Hints     []encryptionKeyHint     `json:"hints"`
}

type rotateDEKRequest struct {
	Service string       `json:"service"`
	All     bool         `json:"all"`
	StepUp  *stepUpProof `json:"step_up,omitempty"`
}

type rotateDEKFailure struct {
	App    string `json:"app"`
	Reason string `json:"reason"`
}

type rotateDEKResponse struct {
	App         string             `json:"app,omitempty"`
	Values      int                `json:"values,omitempty"`
	RotatedAt   *time.Time         `json:"rotated_at,omitempty"`
	RunID       string             `json:"run_id,omitempty"`
	Resumed     bool               `json:"resumed,omitempty"`
	Total       int                `json:"total,omitempty"`
	Rotated     int                `json:"rotated,omitempty"`
	Skipped     int                `json:"skipped,omitempty"`
	FailedCount int                `json:"failed_count,omitempty"`
	Failed      []rotateDEKFailure `json:"failed,omitempty"`
	Complete    bool               `json:"complete"`
}

func (rt *Router) encryptionKeysReady(w http.ResponseWriter) bool {
	if rt.encryptionKeys.rotator == nil || rt.encryptionKeys.policy == nil {
		writeError(w, http.StatusNotImplemented, "key rotation is not configured on this control plane (no master key loaded)")
		return false
	}
	return true
}

// handleGetEncryptionKeys serves GET /api/v1/system/encryption-keys: key
// ages and rotation progress, never key material.
func (rt *Router) handleGetEncryptionKeys(w http.ResponseWriter, r *http.Request) {
	if !rt.encryptionKeysReady(w) {
		return
	}
	res, err := rt.buildEncryptionKeys(r.Context())
	if err != nil {
		rt.internalError(w, "api: encryption keys status failed", err)
		return
	}
	if abs, aerr := rt.callerAbilities(r); aerr != nil || !hasAbility(abs, AbilityRoot) {
		for i := range res.Apps {
			res.Apps[i].ValueCount = 0
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) buildEncryptionKeys(ctx context.Context) (encryptionKeysResource, error) {
	status, err := rt.encryptionKeys.rotator.Status(ctx)
	if err != nil {
		return encryptionKeysResource{}, err
	}
	policy, err := rt.encryptionKeys.policy.GetSecretKeyPolicy(ctx)
	if err != nil {
		return encryptionKeysResource{}, err
	}
	master := masterKeyStatusResource{Configured: rt.masterKeyRotator != nil}
	if rt.masterKeyRotator != nil {
		if at, ok, err := rt.masterKeyRotator.GetMasterKeyRotatedAt(ctx); err == nil && ok {
			days := int(time.Since(at).Hours() / 24)
			master.RotatedAt, master.AgeDays = &at, &days
		}
	}
	res := encryptionKeysResource{
		MasterKey: master,
		Apps:      make([]appKeyResource, 0, len(status.Services)),
		Policy: keyPolicyResource{
			MaxKeyAgeDays: policy.MaxKeyAgeDays, RemindEveryDays: policy.RemindEveryDays,
			LastRemindedAt: timePtr(policy.LastRemindedAt),
		},
		Hints: []encryptionKeyHint{},
	}
	for _, s := range status.Services {
		res.Apps = append(res.Apps, appKeyResource{
			App: s.Service, CreatedAt: s.CreatedAt, RotatedAt: timePtr(s.RotatedAt),
			AgeDays: s.AgeDays, ValueCount: s.ValueCount,
			Overdue: policy.MaxKeyAgeDays > 0 && s.AgeDays > policy.MaxKeyAgeDays,
		})
	}
	if status.LastRun != nil {
		run := status.LastRun
		res.LastRun = &rotationRunResource{ID: run.ID, StartedAt: run.StartedAt, FinishedAt: timePtr(run.FinishedAt),
			Total: run.Total, Done: run.Done, Failed: run.Failed, Actor: run.Actor}
	}
	res.MasterKey.Overdue = master.AgeDays != nil && policy.MaxKeyAgeDays > 0 && *master.AgeDays > policy.MaxKeyAgeDays
	res.Hints = encryptionKeyHints(res, policy.MaxKeyAgeDays)
	return res, nil
}

// encryptionKeyHints turns key ages into typed fixes the dashboard renders as buttons.
func encryptionKeyHints(res encryptionKeysResource, maxAgeDays int) []encryptionKeyHint {
	hints := []encryptionKeyHint{}
	if maxAgeDays <= 0 {
		hints = append(hints, encryptionKeyHint{
			Code: "key_policy_off", Severity: "info",
			Message: "No key age policy is set. Get a reminder every " + strconv.Itoa(store.DefaultMaxKeyAgeDays) + " days to rotate keys.",
			Fix:     encryptionKeyFix{Kind: keyFixEnablePolicy},
		})
	}
	masterLimit := maxAgeDays
	if masterLimit <= 0 {
		masterLimit = defaultMasterKeyHintAgeDays
	}
	if age := res.MasterKey.AgeDays; age != nil && *age > masterLimit {
		sev := "info"
		if res.MasterKey.Overdue {
			sev = "warning"
		}
		hints = append(hints, encryptionKeyHint{
			Code: "master_key_old", Severity: sev,
			Message: "The master key is older than " + strconv.Itoa(masterLimit) + " days.",
			Fix:     encryptionKeyFix{Kind: keyFixRotateMasterKey},
		})
	}
	overdue := 0
	for _, a := range res.Apps {
		if a.Overdue {
			overdue++
			hints = append(hints, encryptionKeyHint{
				Code: "app_key_old", Severity: "warning",
				Message: "The data key for " + a.App + " is older than " + strconv.Itoa(maxAgeDays) + " days.",
				Fix:     encryptionKeyFix{Kind: keyFixRotateApp, App: a.App},
			})
		}
	}
	if overdue > 1 {
		hints = append(hints, encryptionKeyHint{
			Code: "app_keys_old", Severity: "warning",
			Message: strconv.Itoa(overdue) + " app data keys are past the maximum age.",
			Fix:     encryptionKeyFix{Kind: keyFixRotateAll},
		})
	}
	if run := res.LastRun; run != nil && run.FinishedAt == nil {
		hints = append(hints, encryptionKeyHint{
			Code: "rotation_unfinished", Severity: "warning",
			Message: "The last rotation of all data keys was interrupted. Resume it to finish.",
			Fix:     encryptionKeyFix{Kind: keyFixRotateAll},
		})
	}
	return hints
}

type keyPolicyRequest struct {
	MaxKeyAgeDays   *int `json:"max_key_age_days"`
	RemindEveryDays *int `json:"remind_every_days"`
}

// handleUpdateKeyPolicy serves PUT /api/v1/system/encryption-keys/policy.
func (rt *Router) handleUpdateKeyPolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.encryptionKeysReady(w) {
		return
	}
	var req keyPolicyRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	policy, err := rt.encryptionKeys.policy.GetSecretKeyPolicy(r.Context())
	if err != nil {
		rt.internalError(w, "api: load key policy failed", err)
		return
	}
	if req.MaxKeyAgeDays != nil {
		if *req.MaxKeyAgeDays < 0 || *req.MaxKeyAgeDays > maxKeyAgePolicyDays {
			writeError(w, http.StatusBadRequest, "max_key_age_days must be between 0 (off) and 3650")
			return
		}
		policy.MaxKeyAgeDays = *req.MaxKeyAgeDays
	}
	if req.RemindEveryDays != nil {
		if *req.RemindEveryDays < 1 || *req.RemindEveryDays > maxRemindEveryDays {
			writeError(w, http.StatusBadRequest, "remind_every_days must be between 1 and 365")
			return
		}
		policy.RemindEveryDays = *req.RemindEveryDays
	}
	_, _, actor, _ := rt.currentActor(r)
	policy.UpdatedBy = actor
	if err := rt.encryptionKeys.policy.SaveSecretKeyPolicy(r.Context(), policy, time.Now()); err != nil {
		rt.internalError(w, "api: save key policy failed", err)
		return
	}
	res, err := rt.buildEncryptionKeys(r.Context())
	if err != nil {
		rt.internalError(w, "api: encryption keys status failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleRotateDEK serves POST /api/v1/system/encryption-keys/rotate: one
// app's data key (app) or every app's (all), behind a fresh step-up.
func (rt *Router) handleRotateDEK(w http.ResponseWriter, r *http.Request) {
	if !rt.encryptionKeysReady(w) {
		return
	}
	var req rotateDEKRequest
	if err := decodeJSONBodyLimit(r, &req, stepUpBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Service = strings.TrimSpace(req.Service)
	if req.All == (req.Service != "") {
		writeError(w, http.StatusBadRequest, "send either service or all=true")
		return
	}
	userID, ok := rt.keyRotationOwner(w, r, req.StepUp)
	if !ok {
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if actor == "" {
		actor = userID
	}

	if req.All {
		res, err := rt.encryptionKeys.rotator.RotateAll(r.Context(), actor)
		if err != nil {
			rt.writeRotationError(w, err)
			return
		}
		out := rotateDEKResponse{RunID: res.RunID, Resumed: res.Resumed, Total: res.Total, Rotated: res.Rotated,
			Skipped: res.Skipped, FailedCount: res.FailedCount, Complete: res.Complete}
		for _, f := range res.Failed {
			out.Failed = append(out.Failed, rotateDEKFailure{App: f.Service, Reason: f.Reason})
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	res, err := rt.encryptionKeys.rotator.RotateOne(r.Context(), req.Service)
	if err != nil {
		rt.writeRotationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rotateDEKResponse{App: res.Service, Values: res.Values, RotatedAt: &res.RotatedAt, Complete: true})
}

func (rt *Router) writeRotationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, secrets.ErrDEKRotationBusy):
		writeError(w, http.StatusConflict, "a data key rotation is already running")
	case errors.Is(err, store.ErrServiceDEKNotFound):
		writeError(w, http.StatusNotFound, "that app has no data key yet")
	case errors.Is(err, store.ErrDEKRotationConflict):
		writeError(w, http.StatusConflict, "secret values changed during rotation, nothing was changed, retry")
	case errors.Is(err, secrets.ErrDEKUnwrap):
		writeError(w, http.StatusConflict, "the data key does not unwrap under the active master key, nothing was changed")
	default:
		rt.internalError(w, "api: data key rotation failed", err)
	}
}

// keyRotationOwner confirms a fresh step-up: the dashboard's grant for a
// session, or the token owner's password sent inline for a CLI token.
func (rt *Router) keyRotationOwner(w http.ResponseWriter, r *http.Request, proof *stepUpProof) (string, bool) {
	return rt.stepUpOwner(w, r, proof, encryptionKeysStepUpApp, "rotate encryption keys", "key rotation")
}

// stepUpOwner is keyRotationOwner for any manage scope. doing completes
// "confirm it's you to ..." and noun starts "... needs a token that belongs to a user".
func (rt *Router) stepUpOwner(w http.ResponseWriter, r *http.Request, proof *stepUpProof, scopeApp, doing, noun string) (string, bool) {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpSessionOnly, "a session link cannot "+doing)
		return "", false
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		if c, err := r.Cookie(sessionCookieName); err == nil {
			if _, granted := rt.envProtect.grants.check(c.Value, userID, stepUpActionManage, scopeApp, time.Now()); granted {
				return userID, true
			}
		}
		if proof.empty() {
			writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "confirm it's you to "+doing)
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
		writeCodedError(w, http.StatusForbidden, errCodeStepUpSessionOnly, noun+" needs a token that belongs to a user")
		return "", false
	}
	if gone, gerr := rt.tokenOwnerGone(r.Context(), rec); gerr != nil || gone {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return "", false
	}
	if proof.empty() {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "send the token owner's password (and authenticator code) as step_up to "+doing)
		return "", false
	}
	if err := rt.verifyPasswordProof(r, rec.OwnerUserID, *proof); err != nil {
		writeStepUpError(w, err)
		return "", false
	}
	return rec.OwnerUserID, true
}
