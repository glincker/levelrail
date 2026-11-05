package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	silenceForever      = "forever"
	silenceMaxBodyBytes = 8 << 10
	silenceMaxReasonLen = 500
	silenceMaxKeyLen    = 300
	silenceMaxFinite    = 30 * 24 * time.Hour
)

var silenceDurations = map[string]time.Duration{
	"1h":  time.Hour,
	"1d":  24 * time.Hour,
	"1w":  7 * 24 * time.Hour,
	"30d": silenceMaxFinite,
}

// silenceProtectedKinds are issues that signal a security-integrity or
// data-loss risk: muting one always needs an explicit confirmation.
var silenceProtectedKinds = map[string]bool{
	attention.KindLoginAnomaly:            true,
	attention.KindAccountFlagged:          true,
	attention.KindDBSecurityUpdates:       true,
	attention.KindDBUpgradeFailed:         true,
	attention.KindDockerGuard:             true,
	attention.KindSecretRotationFail:      true,
	attention.KindBackupOverdue:           true,
	attention.KindDataCopy:                true,
	attention.KindFirewallAttackSuggested: true,
}

// silenceRootOnlyDoctorKeys are the Docker socket exposure checks by display name
// (either engine) and by code: the dashboard silences by code, an exact match.
var silenceRootOnlyDoctorKeys = map[string]bool{
	"doctor:Docker API guard":       true,
	"doctor:Docker privilege":       true,
	"doctor:Podman API guard":       true,
	"doctor:Podman privilege":       true,
	"doctor:docker_guard":           true,
	"doctor:docker_privilege":       true,
	"doctor:docker_guard_high_risk": true,
	"doctor:Docker API guard alert": true,
	"doctor:Podman API guard alert": true,
}

// silenceNeedsRoot is true for a protected kind or a Docker exposure doctor check.
func silenceNeedsRoot(issueKey string) bool {
	kind, _, _ := strings.Cut(issueKey, ":")
	return silenceProtectedKinds[kind] || silenceRootOnlyDoctorKeys[issueKey]
}

type silenceStore interface {
	CreateSilence(ctx context.Context, s store.Silence) error
	ListSilences(ctx context.Context) ([]store.Silence, error)
	DeleteSilence(ctx context.Context, id string) error
}

func (rt *Router) silenceStore() silenceStore {
	if s, ok := rt.apps.(silenceStore); ok {
		return s
	}
	return nil
}

// activeSilences returns the silences still in force at now, nil when storage is absent.
func (rt *Router) activeSilences(ctx context.Context, now time.Time) []store.Silence {
	s := rt.silenceStore()
	if s == nil {
		return nil
	}
	all, err := s.ListSilences(ctx)
	if err != nil {
		rt.logger.Warn("api: list silences failed", slog.String("error", err.Error()))
		return nil
	}
	all = capProtectedSilences(all, rt.protectedSilenceMax(ctx))
	out := all[:0]
	for _, sil := range all {
		if sil.Active(now) {
			out = append(out, sil)
		}
	}
	return out
}

func (rt *Router) registerSilenceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/silences", rt.requireAbility(AbilityRead, rt.handleListSilences))
	mux.HandleFunc("POST /api/v1/silences", rt.requireAbility(AbilityWrite, rt.handleCreateSilence))
	mux.HandleFunc("DELETE /api/v1/silences/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteSilence))
}

type issueSilenceResource struct {
	store.Silence
	Expired bool `json:"expired"`
}

type issueSilenceListResponse struct {
	Silences []issueSilenceResource `json:"silences"`
	// ProtectedMaxDays and ProtectedKeys let a client hide durations that would fail.
	ProtectedMaxDays int      `json:"protected_max_days"`
	ProtectedKeys    []string `json:"protected_keys"`
}

// protectedSilenceMax is the longest a protected issue may stay silenced; 0 means no cap.
func (rt *Router) protectedSilenceMax(ctx context.Context) time.Duration {
	return time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeySecurityProtectedSilenceMaxDays)) * 24 * time.Hour
}

// protectedSilenceKeys lists the protected kinds and exact issue keys.
func protectedSilenceKeys() []string {
	out := make([]string, 0, len(silenceProtectedKinds)+len(silenceRootOnlyDoctorKeys))
	for k := range silenceProtectedKinds {
		out = append(out, k)
	}
	for k := range silenceRootOnlyDoctorKeys {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// capProtectedSilences ends a protected silence at CreatedAt plus limit, so one
// made before the cap existed (or forever) stops hiding its issue.
func capProtectedSilences(all []store.Silence, limit time.Duration) []store.Silence {
	if limit <= 0 {
		return all
	}
	out := slices.Clone(all)
	for i, s := range out {
		if !silenceNeedsRoot(s.IssueKey) {
			continue
		}
		if end := s.CreatedAt.Add(limit); s.Until == nil || s.Until.After(end) {
			out[i].Until = &end
		}
	}
	return out
}

// handleListSilences handles GET /api/v1/silences: every silence, with
// expiry decided now.
func (rt *Router) handleListSilences(w http.ResponseWriter, r *http.Request) {
	s := rt.silenceStore()
	if s == nil {
		writeJSON(w, http.StatusOK, issueSilenceListResponse{Silences: []issueSilenceResource{}, ProtectedKeys: protectedSilenceKeys()})
		return
	}
	all, err := s.ListSilences(r.Context())
	if err != nil {
		rt.internalError(w, "api: list silences", err)
		return
	}
	if all, err = rt.filterSilences(r, all); err != nil {
		rt.internalError(w, "api: list silences: filter", err)
		return
	}
	capDur := rt.protectedSilenceMax(r.Context())
	all = capProtectedSilences(all, capDur)
	now := time.Now()
	out := make([]issueSilenceResource, 0, len(all))
	for _, sil := range all {
		out = append(out, issueSilenceResource{Silence: sil, Expired: !sil.Active(now)})
	}
	writeJSON(w, http.StatusOK, issueSilenceListResponse{Silences: out, ProtectedMaxDays: int(capDur / (24 * time.Hour)), ProtectedKeys: protectedSilenceKeys()})
}

type createIssueSilenceRequest struct {
	IssueKey string `json:"issue_key"`
	Scope    string `json:"scope"`
	Duration string `json:"duration"`
	Reason   string `json:"reason"`
	Severity string `json:"severity"`
	Confirm  bool   `json:"confirm"`
}

var errSilenceConfirm = errors.New("silencing this issue needs confirmation")

var errSilenceTooLong = errors.New("silence is longer than allowed for a security issue")

// silenceUntil resolves a duration to an expiry. A protected issue needs
// confirm and lasts at most protectedMax (0 means no cap); forever on a critical one needs confirm too.
func silenceUntil(req createIssueSilenceRequest, now time.Time, protectedMax time.Duration) (*time.Time, error) {
	protected := silenceNeedsRoot(req.IssueKey)
	if protected && !req.Confirm {
		return nil, errSilenceConfirm
	}
	if protected && protectedMax > 0 && (req.Duration == silenceForever || silenceDurations[req.Duration] > protectedMax) {
		return nil, errSilenceTooLong
	}
	if req.Duration == silenceForever {
		if (protected || req.Severity == attention.Critical) && !req.Confirm {
			return nil, errSilenceConfirm
		}
		return nil, nil
	}
	d, ok := silenceDurations[req.Duration]
	if !ok {
		return nil, errors.New("duration must be one of 1h, 1d, 1w, 30d, forever")
	}
	until := now.Add(d)
	return &until, nil
}

// handleCreateSilence handles POST /api/v1/silences. The issue key is a
// status item id, not a resource, so only a named scope is a resource ref.
func (rt *Router) handleCreateSilence(w http.ResponseWriter, r *http.Request) {
	s := rt.silenceStore()
	if s == nil {
		writeError(w, http.StatusNotImplemented, "silences storage is not configured")
		return
	}
	var req createIssueSilenceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, silenceMaxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.IssueKey, req.Scope, req.Reason = strings.TrimSpace(req.IssueKey), strings.TrimSpace(req.Scope), strings.TrimSpace(req.Reason)
	if req.IssueKey == "" || len(req.IssueKey) > silenceMaxKeyLen || len(req.Reason) > silenceMaxReasonLen {
		writeError(w, http.StatusBadRequest, "issue_key is required and reason must be under 500 characters")
		return
	}
	if silenceNeedsRoot(req.IssueKey) {
		if abs, aerr := rt.callerAbilities(r); aerr != nil || !hasAbility(abs, AbilityRoot) {
			writeError(w, http.StatusForbidden, "silencing this issue needs the root ability")
			return
		}
	}
	now := time.Now()
	protectedMax := rt.protectedSilenceMax(r.Context())
	until, err := silenceUntil(req, now, protectedMax)
	if errors.Is(err, errSilenceTooLong) {
		writeError(w, http.StatusBadRequest, "a security issue can be silenced for at most "+strconv.Itoa(int(protectedMax/(24*time.Hour)))+" days, see the setting "+runtimesettings.KeySecurityProtectedSilenceMaxDays)
		return
	}
	if errors.Is(err, errSilenceConfirm) {
		writeError(w, http.StatusConflict, err.Error()+": resend with confirm set")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Scope != "" && !rt.authorizeRefs(w, r, AbilityWrite, refDenial{}, appRef(req.Scope)) {
		return
	}
	id, err := store.NewSilenceID()
	if err != nil {
		rt.internalError(w, "api: create silence id", err)
		return
	}
	sil := store.Silence{
		ID: id, IssueKey: req.IssueKey, Scope: req.Scope, Until: until, Severity: req.Severity,
		CreatedBy: rt.eventActor(r), Reason: req.Reason, CreatedAt: now,
	}
	if err := s.CreateSilence(r.Context(), sil); err != nil {
		rt.internalError(w, "api: create silence", err)
		return
	}
	writeJSON(w, http.StatusCreated, issueSilenceResource{Silence: sil})
}

// handleDeleteSilence handles DELETE /api/v1/silences/{id}: the issue shows again.
func (rt *Router) handleDeleteSilence(w http.ResponseWriter, r *http.Request) {
	s := rt.silenceStore()
	if s == nil {
		writeError(w, http.StatusNotImplemented, "silences storage is not configured")
		return
	}
	id := r.PathValue("id")
	all, err := s.ListSilences(r.Context())
	if err == nil {
		all, err = rt.filterSilences(r, all)
	}
	if err != nil {
		rt.internalError(w, "api: delete silence: scope", err)
		return
	}
	if !slices.ContainsFunc(all, func(x store.Silence) bool { return x.ID == id }) {
		writeError(w, http.StatusNotFound, "no such silence")
		return
	}
	err = s.DeleteSilence(r.Context(), id)
	if errors.Is(err, store.ErrSilenceNotFound) {
		writeError(w, http.StatusNotFound, "no such silence")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete silence", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// withoutSilenced drops attention items an active silence covers.
func withoutSilenced(items []attention.Item, silences []store.Silence, now time.Time) []attention.Item {
	if len(silences) == 0 {
		return items
	}
	out := items[:0]
	for _, it := range items {
		if _, muted := store.FindSilence(silences, it.ID, "", now); !muted {
			out = append(out, it)
		}
	}
	return out
}

// silencedDoctorCount counts failing or warning checks an active silence covers.
func silencedDoctorCount(checks []doctorCheckResource, silences []store.Silence, now time.Time) int {
	n := 0
	for _, c := range checks {
		if c.Status != "warn" && c.Status != "fail" {
			continue
		}
		if _, muted := store.FindSilence(silences, "doctor:"+c.Name, "", now); muted {
			n++
		}
	}
	return n
}
