package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/abuse"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	abuseReasonPending  = "Pending"
	abuseReasonOff      = "Off"
	abuseCountersWindow = 15 * time.Minute
	abusePeakWindow     = time.Hour
	abusePeakStep       = time.Minute
	abuseDryRunNote     = "Preview only. Nothing was blocked or changed."
	maxAbuseBody        = 64 << 10
)

// abuseStore is the optional slice of the app store the abuse routes need.
type abuseStore interface {
	GetAppAbusePolicy(ctx context.Context, app string) (store.AppAbusePolicy, bool, error)
	SetAppAbusePolicy(ctx context.Context, app string, spec []byte, actor string) error
}

// AccessReplaySource lists every retained edge request of an app since a time,
// oldest first, for the abuse dry run.
type AccessReplaySource interface {
	AppAccessSince(app string, since time.Time) []ingress.AccessRecord
}

// AppAccessSince implements AccessReplaySource.
func (e EdgeAccess) AppAccessSince(app string, since time.Time) []ingress.AccessRecord {
	return e.Log.Since(e.Stats.KeysForApp(app), since)
}

type abuseCondition struct {
	Status  string    `json:"status"`
	Reason  string    `json:"reason"`
	Message string    `json:"message"`
	Since   time.Time `json:"since,omitempty"`
}

type abuseReasonCount struct {
	Reason     string  `json:"reason"`
	WouldBlock float64 `json:"would_block"`
	Blocked    float64 `json:"blocked"`
}

type abuseLimitsResource struct {
	MaxRPS         float64 `json:"max_rps"`
	MaxBurst       int     `json:"max_burst"`
	MaxDenyEntries int     `json:"max_deny_entries"`
	MaxAgentEntry  int     `json:"max_agent_entries"`
	DryRunDefault  int     `json:"dry_run_default_minutes"`
	DryRunMax      int     `json:"dry_run_max_minutes"`
}

type abuseResource struct {
	App        string           `json:"app"`
	Configured bool             `json:"configured"`
	Policy     abuse.Policy     `json:"policy"`
	UpdatedAt  string           `json:"updated_at,omitempty"`
	UpdatedBy  string           `json:"updated_by,omitempty"`
	Condition  abuseCondition   `json:"condition"`
	Suggestion abuse.Suggestion `json:"suggestion"`
	Counters   trafficAbuse     `json:"counters"`
	// CountersWindowMinutes is the span Counters covers.
	CountersWindowMinutes int                 `json:"counters_window_minutes"`
	Limits                abuseLimitsResource `json:"limits"`
}

type abuseDryRunRequest struct {
	// Policy is the draft to replay; omitted means the saved policy.
	Policy  *abuse.Policy `json:"policy,omitempty"`
	Minutes int           `json:"minutes,omitempty"`
}

type abuseDryRunResource struct {
	abuse.ReplayResult
	WindowMinutes float64 `json:"window_minutes"`
	// CoveredMinutes is how much of the window the in-memory access log still
	// holds; shorter than WindowMinutes means the log wrapped or restarted.
	CoveredMinutes float64 `json:"covered_minutes"`
	Note           string  `json:"note"`
}

type abuseValidationResponse struct {
	Error  string             `json:"error"`
	Fields []abuse.FieldError `json:"fields"`
}

func (rt *Router) abuseStore() abuseStore {
	st, _ := rt.apps.(abuseStore)
	return st
}

func abuseLimits() abuse.Limits { return abuse.LimitsFromEnv(os.Getenv) }

func newAbuseLimitsResource(l abuse.Limits) abuseLimitsResource {
	return abuseLimitsResource{
		MaxRPS: l.MaxRPS, MaxBurst: l.MaxBurst, MaxDenyEntries: l.MaxDenyEntries, MaxAgentEntry: l.MaxAgentEntries,
		DryRunDefault: int(l.DryRunDefault.Minutes()), DryRunMax: int(l.DryRunMax.Minutes()),
	}
}

// abuseAppExists answers 404 for an unknown app and reports whether to go on.
func (rt *Router) abuseAppExists(w http.ResponseWriter, r *http.Request, name string) bool {
	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return false
	}
	if err != nil {
		rt.internalError(w, "api: abuse defense: load app failed", err, slog.String("name", name))
		return false
	}
	return true
}

func (rt *Router) abuseResource(ctx context.Context, name string) (abuseResource, error) {
	l := abuseLimits()
	res := abuseResource{
		App: name, Policy: abuse.Policy{Mode: abuse.ModeOff}.Normalize(l),
		Condition: abuseCondition{Status: "False", Reason: abuseReasonOff, Message: "No policy is set. The defense is off."},
		Limits:    newAbuseLimitsResource(l),
	}
	if st := rt.abuseStore(); st != nil {
		row, found, err := st.GetAppAbusePolicy(ctx, name)
		if err != nil {
			return abuseResource{}, err
		}
		if found {
			var p abuse.Policy
			if err := json.Unmarshal(row.Spec, &p); err != nil {
				return abuseResource{}, err
			}
			res.Configured, res.Policy = true, p.Normalize(l)
			res.UpdatedAt, res.UpdatedBy = row.UpdatedAt, row.UpdatedBy
			res.Condition = abuseConditionOf(res.Policy, row)
		}
	}
	now := time.Now()
	res.CountersWindowMinutes = int(abuseCountersWindow.Minutes())
	res.Counters = emptyAbuseTraffic()
	if rt.telemetry != nil {
		res.Counters = abuseTraffic(ctx, rt.telemetry, name, now.Add(-abuseCountersWindow), now)
	}
	res.Suggestion = abuse.Suggest(rt.peakRPS(ctx, name, now), l)
	return res, nil
}

func abuseConditionOf(p abuse.Policy, row store.AppAbusePolicy) abuseCondition {
	if row.ConditionReason == "" {
		if !p.Active() {
			return abuseCondition{Status: "False", Reason: abuseReasonOff, Message: "The defense is off."}
		}
		return abuseCondition{Status: "Unknown", Reason: abuseReasonPending, Message: "Waiting for the ingress to pick up the policy."}
	}
	c := abuseCondition{Status: row.ConditionStatus, Reason: row.ConditionReason, Message: row.ConditionMessage}
	if t, err := time.Parse(time.RFC3339, row.ConditionAt); err == nil {
		c.Since = t
	}
	return c
}

// peakRPS is the busiest minute of the last hour, 0 without traffic data.
func (rt *Router) peakRPS(ctx context.Context, app string, now time.Time) float64 {
	if rt.telemetry == nil {
		return 0
	}
	points, _ := telemetry.QueryRequests(ctx, rt.telemetry, app, now.Add(-abusePeakWindow), now, abusePeakStep)
	var peak float64
	for _, p := range points {
		peak = max(peak, p.RatePerSec)
	}
	return peak
}

// handleGetAppAbuse handles GET /api/v1/apps/{name}/abuse.
func (rt *Router) handleGetAppAbuse(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	res, err := rt.abuseResource(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: abuse defense: read failed", err, slog.String("name", name))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

func writeAbuseValidation(w http.ResponseWriter, err error) {
	var ve *abuse.ValidationError
	if errors.As(err, &ve) {
		writeJSON(w, http.StatusBadRequest, abuseValidationResponse{Error: ve.Error(), Fields: ve.Fields})
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

// handlePutAppAbuse handles PUT /api/v1/apps/{name}/abuse: it replaces the
// app's policy and nudges the ingress, which applies it on its next pass.
func (rt *Router) handlePutAppAbuse(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	st := rt.abuseStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "abuse defense is not available")
		return
	}
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	var p abuse.Policy
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAbuseBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	l := abuseLimits()
	p = p.Normalize(l)
	if err := p.Validate(l); err != nil {
		writeAbuseValidation(w, err)
		return
	}
	raw, err := json.Marshal(p)
	if err != nil {
		rt.internalError(w, "api: abuse defense: encode failed", err, slog.String("name", name))
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if err := st.SetAppAbusePolicy(r.Context(), name, raw, actor); err != nil {
		rt.internalError(w, "api: abuse defense: save failed", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	res, err := rt.abuseResource(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: abuse defense: read back failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleAbuseDryRun handles POST /api/v1/apps/{name}/abuse/dry-run: it replays
// the recent edge access log against a draft policy and reports what it would
// have blocked. It reads memory only and changes nothing.
func (rt *Router) handleAbuseDryRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	src, ok := rt.access.(AccessReplaySource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return
	}
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	var req abuseDryRunRequest
	if err := decodeJSONBodyLimit(r, &req, maxAbuseBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	l := abuseLimits()
	window := l.DryRunDefault
	if req.Minutes < 0 || time.Duration(req.Minutes)*time.Minute > l.DryRunMax {
		writeError(w, http.StatusBadRequest, "minutes must be between 1 and "+strconv.Itoa(int(l.DryRunMax.Minutes())))
		return
	}
	if req.Minutes > 0 {
		window = time.Duration(req.Minutes) * time.Minute
	}
	draft, err := rt.abuseDraft(r.Context(), name, req.Policy, l)
	if err != nil {
		writeAbuseValidation(w, err)
		return
	}
	since := time.Now().Add(-window)
	recs := src.AppAccessSince(name, since)
	obs := make([]abuse.Observation, len(recs))
	for i, rec := range recs {
		obs[i] = abuse.Observation{Time: rec.Time, Client: rec.Client, UserAgent: rec.UserAgent}
	}
	res, err := abuse.Replay(draft, obs, l, l.DryRunTop)
	if err != nil {
		writeAbuseValidation(w, err)
		return
	}
	out := abuseDryRunResource{ReplayResult: res, WindowMinutes: window.Minutes(), Note: abuseDryRunNote}
	if res.Requests > 1 {
		out.CoveredMinutes = res.To.Sub(res.From).Minutes()
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// abuseDraft is the policy to replay: the submitted draft, else the saved one.
func (rt *Router) abuseDraft(ctx context.Context, name string, draft *abuse.Policy, l abuse.Limits) (abuse.Policy, error) {
	if draft != nil {
		return draft.Normalize(l), nil
	}
	if st := rt.abuseStore(); st != nil {
		if row, found, err := st.GetAppAbusePolicy(ctx, name); err == nil && found {
			var p abuse.Policy
			if json.Unmarshal(row.Spec, &p) == nil {
				return p.Normalize(l), nil
			}
		}
	}
	return abuse.Policy{}, &abuse.ValidationError{Fields: []abuse.FieldError{{Field: "policy", Message: "send a draft policy; the app has none saved"}}}
}
