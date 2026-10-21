package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
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
}

// handleListSilences handles GET /api/v1/silences: every silence, with
// expiry decided now.
func (rt *Router) handleListSilences(w http.ResponseWriter, r *http.Request) {
	s := rt.silenceStore()
	if s == nil {
		writeJSON(w, http.StatusOK, issueSilenceListResponse{Silences: []issueSilenceResource{}})
		return
	}
	all, err := s.ListSilences(r.Context())
	if err != nil {
		rt.internalError(w, "api: list silences", err)
		return
	}
	now := time.Now()
	out := make([]issueSilenceResource, 0, len(all))
	for _, sil := range all {
		out = append(out, issueSilenceResource{Silence: sil, Expired: !sil.Active(now)})
	}
	writeJSON(w, http.StatusOK, issueSilenceListResponse{Silences: out})
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

// silenceUntil resolves a duration to an expiry, enforcing the safety rules:
// a protected or critical issue needs confirm to be silenced forever, and a
// protected issue needs it for any duration.
func silenceUntil(req createIssueSilenceRequest, now time.Time) (*time.Time, error) {
	kind, _, _ := strings.Cut(req.IssueKey, ":")
	protected := silenceProtectedKinds[kind]
	if protected && !req.Confirm {
		return nil, errSilenceConfirm
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
	now := time.Now()
	until, err := silenceUntil(req, now)
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
	err := s.DeleteSilence(r.Context(), r.PathValue("id"))
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
