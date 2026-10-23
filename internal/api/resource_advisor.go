package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	advisorDefaultSnooze = 7 * 24 * time.Hour
	advisorMaxSnooze     = 90 * 24 * time.Hour
	advisorTopSavings    = 5
	advisorRefreshBudget = 30 * time.Second
)

// advisorStateStore persists dismissals, snoozes and the beta switch.
type advisorStateStore interface {
	SetAdvisorFindingState(ctx context.Context, s store.AdvisorFindingState) error
	ClearAdvisorFindingState(ctx context.Context, findingID string) error
	ListAdvisorFindingStates(ctx context.Context) (map[string]store.AdvisorFindingState, error)
	SetBetaFeature(ctx context.Context, feature string, enabled bool, actor string, now time.Time) error
	ListBetaFeatures(ctx context.Context) (map[string]bool, error)
}

func (rt *Router) advisorStore() advisorStateStore {
	if s, ok := rt.apps.(advisorStateStore); ok {
		return s
	}
	return nil
}

// advisorFindingResource is a finding plus the caller's view of it.
type advisorFindingResource struct {
	resourceadvisor.Finding
	State        string `json:"state,omitempty"`
	SnoozedUntil string `json:"snoozed_until,omitempty"`
	Appliable    bool   `json:"appliable"`
	CanApply     bool   `json:"can_apply"`
}

type advisorFindingsResponse struct {
	GeneratedAt string                   `json:"generated_at"`
	Findings    []advisorFindingResource `json:"findings"`
}

type advisorSummaryResponse struct {
	GeneratedAt string `json:"generated_at"`
	resourceadvisor.Summary
}

// advisorAccess answers what the caller may see and change.
type advisorAccess struct {
	canRead  func(resourceadvisor.Ref) bool
	canWrite func(resourceadvisor.Ref, string) bool
}

func (rt *Router) advisorAccessFor(r *http.Request) (advisorAccess, error) {
	abilities, policies, scope, err := rt.callerScope(r)
	if err != nil {
		return advisorAccess{}, err
	}
	check := func(ref resourceadvisor.Ref, ability string) bool {
		switch ref.Kind {
		case resourceadvisor.ResourceApp:
			return scope.authorize(abilities, policies, ability, resourcePrefixApp, ref.Name)
		case resourceadvisor.ResourceDatabase:
			return scope.authorize(abilities, policies, ability, resourcePrefixDatabase, ref.Name)
		}
		return hasAbility(abilities, AbilityRoot)
	}
	return advisorAccess{
		canRead:  func(ref resourceadvisor.Ref) bool { return check(ref, AbilityRead) },
		canWrite: check,
	}, nil
}

// applyAbility is what changing a finding's resource needs.
func applyAbility(f resourceadvisor.Finding) string {
	if f.Action != nil && f.Action.Type == resourceadvisor.ActionEnableSleep {
		return AbilityDeploy
	}
	return AbilityWrite
}

// visibleAdvisorFindings filters the cached analysis to the caller and
// annotates dismissal state. hidden findings are dropped unless asked for.
func (rt *Router) visibleAdvisorFindings(r *http.Request, all []resourceadvisor.Finding, includeHidden bool) ([]advisorFindingResource, error) {
	access, err := rt.advisorAccessFor(r)
	if err != nil {
		return nil, err
	}
	states := map[string]store.AdvisorFindingState{}
	if s := rt.advisorStore(); s != nil {
		if states, err = s.ListAdvisorFindingStates(r.Context()); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	out := []advisorFindingResource{}
	for _, f := range all {
		if !access.canRead(f.Resource) {
			continue
		}
		st, has := states[f.ID]
		hidden := has && st.Hides(now)
		if hidden && !includeHidden {
			continue
		}
		fr := advisorFindingResource{Finding: f, Appliable: f.Action != nil && f.Action.Appliable()}
		fr.CanApply = fr.Appliable && access.canWrite(f.Resource, applyAbility(f))
		if hidden {
			fr.State = st.State
			if st.State == store.AdvisorFindingSnoozed {
				fr.SnoozedUntil = st.SnoozedUntil.UTC().Format(time.RFC3339)
			}
		}
		out = append(out, fr)
	}
	return out, nil
}

func (rt *Router) advisorGeneratedAt() string {
	rt.advisor.mu.Lock()
	defer rt.advisor.mu.Unlock()
	return rt.advisor.at.UTC().Format(time.RFC3339)
}

// handleResourceAdvisorFindings serves GET /api/v1/resource-advisor/findings.
func (rt *Router) handleResourceAdvisorFindings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var ref *resourceadvisor.Ref
	if v := q.Get("resource"); v != "" {
		parsed, err := resourceadvisor.ParseRef(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ref = &parsed
	}
	severity := resourceadvisor.Severity(q.Get("severity"))
	switch severity {
	case "", resourceadvisor.SeverityInfo, resourceadvisor.SeveritySuggest, resourceadvisor.SeverityWarn:
	default:
		writeError(w, http.StatusBadRequest, "severity must be info, suggest or warn")
		return
	}
	all, err := rt.advisorFindings(r.Context())
	if err != nil {
		rt.internalError(w, "api: resource advisor findings", err)
		return
	}
	visible, err := rt.visibleAdvisorFindings(r, all, q.Get("include") == "hidden")
	if err != nil {
		rt.internalError(w, "api: resource advisor visibility", err)
		return
	}
	out := []advisorFindingResource{}
	for _, f := range visible {
		if ref != nil && f.Resource != *ref {
			continue
		}
		if severity != "" && f.Severity.Rank() < severity.Rank() {
			continue
		}
		if ref == nil {
			f.Evidence.Series = nil
		}
		out = append(out, f)
	}
	writeJSON(w, http.StatusOK, advisorFindingsResponse{GeneratedAt: rt.advisorGeneratedAt(), Findings: out})
}

// handleResourceAdvisorSummary serves GET /api/v1/resource-advisor/summary.
func (rt *Router) handleResourceAdvisorSummary(w http.ResponseWriter, r *http.Request) {
	all, err := rt.advisorFindings(r.Context())
	if err != nil {
		rt.internalError(w, "api: resource advisor summary", err)
		return
	}
	visible, err := rt.visibleAdvisorFindings(r, all, false)
	if err != nil {
		rt.internalError(w, "api: resource advisor visibility", err)
		return
	}
	plain := make([]resourceadvisor.Finding, 0, len(visible))
	var inScope func(resourceadvisor.Ref) bool
	if requestScoped(r) {
		if inScope, err = rt.scopedAdvisorRefs(r); err != nil {
			rt.internalError(w, "api: resource advisor scope", err)
			return
		}
	}
	for _, f := range visible {
		if inScope == nil || inScope(f.Resource) {
			plain = append(plain, f.Finding)
		}
	}
	writeJSON(w, http.StatusOK, advisorSummaryResponse{GeneratedAt: rt.advisorGeneratedAt(), Summary: resourceadvisor.Summarize(plain, advisorTopSavings)})
}

var errAdvisorFindingNotFound = errors.New("finding not found")

// advisorFindingForWrite loads a visible finding the caller may change.
func (rt *Router) advisorFindingForWrite(w http.ResponseWriter, r *http.Request) (resourceadvisor.Finding, bool) {
	id := r.PathValue("id")
	all, err := rt.advisorFindings(r.Context())
	if err != nil {
		rt.internalError(w, "api: resource advisor findings", err)
		return resourceadvisor.Finding{}, false
	}
	access, err := rt.advisorAccessFor(r)
	if err != nil {
		rt.internalError(w, "api: resource advisor access", err)
		return resourceadvisor.Finding{}, false
	}
	for _, f := range all {
		if f.ID != id || !access.canRead(f.Resource) {
			continue
		}
		if !access.canWrite(f.Resource, applyAbility(f)) {
			writeError(w, http.StatusForbidden, "you cannot change "+f.Resource.String())
			return resourceadvisor.Finding{}, false
		}
		return f, true
	}
	writeError(w, http.StatusNotFound, errAdvisorFindingNotFound.Error())
	return resourceadvisor.Finding{}, false
}

type advisorSnoozeRequest struct {
	Duration string `json:"duration"`
}

// handleResourceAdvisorState serves dismiss, snooze and restore.
func (rt *Router) handleResourceAdvisorState(state string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := rt.advisorStore()
		if s == nil {
			writeError(w, http.StatusNotImplemented, "advisor state storage is not configured")
			return
		}
		f, ok := rt.advisorFindingForWrite(w, r)
		if !ok {
			return
		}
		now := time.Now()
		rec := store.AdvisorFindingState{FindingID: f.ID, State: state, Actor: rt.eventActor(r), UpdatedAt: now}
		if state == store.AdvisorFindingSnoozed {
			var req advisorSnoozeRequest
			if r.ContentLength != 0 {
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeError(w, http.StatusBadRequest, "invalid request body")
					return
				}
			}
			d := advisorDefaultSnooze
			if strings.TrimSpace(req.Duration) != "" {
				parsed, err := parseAdvisorDuration(req.Duration)
				if err != nil || parsed <= 0 || parsed > advisorMaxSnooze {
					writeError(w, http.StatusBadRequest, "duration must be between 1m and 90d, such as 24h or 7d")
					return
				}
				d = parsed
			}
			rec.SnoozedUntil = now.Add(d)
		}
		var err error
		if state == "" {
			err = s.ClearAdvisorFindingState(r.Context(), f.ID)
		} else {
			err = s.SetAdvisorFindingState(r.Context(), rec)
		}
		if err != nil {
			rt.internalError(w, "api: resource advisor state", err)
			return
		}
		resp := advisorFindingResource{Finding: f, State: state, Appliable: f.Action != nil && f.Action.Appliable()}
		if state == store.AdvisorFindingSnoozed {
			resp.SnoozedUntil = rec.SnoozedUntil.UTC().Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// parseAdvisorDuration accepts Go durations and whole days ("7d").
func parseAdvisorDuration(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if n, ok := strings.CutSuffix(v, "d"); ok {
		d, err := time.ParseDuration(n + "h")
		return d * 24, err
	}
	return time.ParseDuration(v)
}

// advisorRefreshing guards the one background refresh the attention feed
// may start, so a burst of polls never stacks refreshes.
var advisorRefreshing atomic.Bool

// advisorFindingsCached never computes inline: it returns what is cached
// and refreshes in the background when that is stale.
func (rt *Router) advisorFindingsCached() []resourceadvisor.Finding {
	c := rt.advisor
	c.mu.Lock()
	findings, at := c.findings, c.at
	c.mu.Unlock()
	if (at.IsZero() || time.Since(at) >= rt.advisorThresholds().CacheTTL) && advisorRefreshing.CompareAndSwap(false, true) {
		go func() {
			defer advisorRefreshing.Store(false)
			ctx, cancel := context.WithTimeout(context.Background(), advisorRefreshBudget)
			defer cancel()
			if _, err := rt.advisorFindings(ctx); err != nil {
				rt.logger.Warn("resource advisor: background refresh failed", slog.String("error", err.Error()))
			}
		}()
	}
	return findings
}
