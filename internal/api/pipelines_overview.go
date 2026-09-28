package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// PipelineOverviewStore is the cross-app read surface behind the Pipelines
// overview. *store.DB satisfies it.
type PipelineOverviewStore interface {
	ListPipelineRunRows(ctx context.Context, f store.PipelineRunFilter) ([]store.PipelineRunRow, error)
	PipelineAppCounts(ctx context.Context, since time.Time) ([]store.PipelineAppCounts, error)
}

const (
	overviewDefaultLimit = 50
	overviewMaxLimit     = 200
	// overviewMaxBatches bounds how many store pages one request scans while
	// per-row access filtering discards rows.
	overviewMaxBatches = 10
	shortSHALen        = 7
)

var (
	overviewStatuses = map[string]bool{"running": true, "failed": true, "succeeded": true, "cancelled": true, "waiting_approval": true, "held": true}
	overviewTriggers = map[string]bool{"push": true, "pull_request": true, "tag": true, "manual": true, "schedule": true, "api": true}
)

type pipelineRunRowResource struct {
	ID              string     `json:"id"`
	App             string     `json:"app"`
	Pipeline        string     `json:"pipeline"`
	Number          int        `json:"number"`
	Status          string     `json:"status"`
	Reason          string     `json:"reason,omitempty"`
	Trigger         string     `json:"trigger"`
	Ref             string     `json:"ref,omitempty"`
	ShortSHA        string     `json:"short_sha,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	DurationSeconds *int64     `json:"duration_seconds,omitempty"`
	ApprovalPending bool       `json:"approval_pending"`
	ApprovalID      int64      `json:"approval_id,omitempty"`
	HoldPending     bool       `json:"hold_pending"`
	CanDecide       bool       `json:"can_decide"`
}

type pipelineRunRowsPage struct {
	Runs       []pipelineRunRowResource `json:"runs"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

type pipelineSummaryResource struct {
	Running         int      `json:"running"`
	Succeeded24h    int      `json:"succeeded_24h"`
	Failed24h       int      `json:"failed_24h"`
	Cancelled24h    int      `json:"cancelled_24h"`
	WaitingApproval int      `json:"waiting_approval"`
	Held            int      `json:"held"`
	SuccessRate24h  *float64 `json:"success_rate_24h"`
}

// callerAccess resolves a request's abilities and IAM policies once so each
// row can be checked without another lookup.
type callerAccess struct {
	abilities []string
	policies  []store.Policy
}

func (a callerAccess) can(ability, app string) bool {
	return authorizeResource(a.abilities, a.policies, ability, "app:"+app)
}

func (rt *Router) resolveCallerAccess(r *http.Request) (callerAccess, error) {
	var principalType, principalID string
	var abilities []string
	if userID, ok := rt.currentSessionUserID(r); ok {
		user, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			return callerAccess{}, fmt.Errorf("api: load session user: %w", err)
		}
		principalType, principalID, abilities = store.PrincipalTypeUser, userID, user.Abilities
	} else if tok, ok := bearerToken(r); ok {
		rec, err := rt.tokens.GetAPITokenByHash(r.Context(), hashToken(tok))
		if err != nil {
			return callerAccess{}, fmt.Errorf("api: load api token: %w", err)
		}
		principalType, principalID, abilities = store.PrincipalTypeToken, rec.ID, rec.Abilities
	} else {
		return callerAccess{}, fmt.Errorf("api: no caller identity")
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		return callerAccess{}, fmt.Errorf("api: load caller policies: %w", err)
	}
	return callerAccess{abilities: abilities, policies: policies}, nil
}

func (rt *Router) overviewStore(w http.ResponseWriter) (PipelineOverviewStore, bool) {
	if !rt.pipelinesReady(w) {
		return nil, false
	}
	ov, ok := rt.pipelineStore.(PipelineOverviewStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "pipeline overview is not available on this control plane")
		return nil, false
	}
	return ov, true
}

func encodeRunCursor(created time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(created.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeRunCursor(s string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor: %w", err)
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok || id == "" {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse cursor time: %w", err)
	}
	return t, id, nil
}

func toRunRowResource(row store.PipelineRunRow, access callerAccess, now time.Time) pipelineRunRowResource {
	res := pipelineRunRowResource{
		ID: row.ID, App: row.AppName, Pipeline: row.PipelineName, Number: row.Number, Status: row.Status, Reason: row.Reason,
		Trigger: row.TriggerKind, Ref: row.Ref, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt,
		ApprovalPending: row.Status == store.PipelineStatusWaitingApproval,
		ApprovalID:      row.PendingApprovalID,
		HoldPending:     row.HoldState == store.HoldPending,
	}
	if len(row.CommitSHA) >= shortSHALen {
		res.ShortSHA = row.CommitSHA[:shortSHALen]
	} else {
		res.ShortSHA = row.CommitSHA
	}
	if row.StartedAt != nil {
		end := now
		if row.FinishedAt != nil {
			end = *row.FinishedAt
		}
		d := int64(end.Sub(*row.StartedAt).Seconds())
		if d < 0 {
			d = 0
		}
		res.DurationSeconds = &d
	}
	switch {
	case res.HoldPending:
		res.CanDecide = access.can(AbilityDeploy, row.AppName)
	case res.ApprovalPending && row.PendingApprovalID != 0:
		res.CanDecide = access.can(AbilityDeploy, row.AppName) && access.can(row.ApprovalAbility, row.AppName)
	}
	return res
}

// handleListAllPipelineRuns handles GET /api/v1/pipeline-runs.
func (rt *Router) handleListAllPipelineRuns(w http.ResponseWriter, r *http.Request) {
	ov, ok := rt.overviewStore(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	filter := store.PipelineRunFilter{Status: q.Get("status"), App: q.Get("app"), Pipeline: q.Get("pipeline"), Trigger: q.Get("trigger")}
	if filter.Status != "" && !overviewStatuses[filter.Status] {
		writeError(w, http.StatusBadRequest, "unknown status filter")
		return
	}
	if filter.Trigger != "" && !overviewTriggers[filter.Trigger] {
		writeError(w, http.StatusBadRequest, "unknown trigger filter")
		return
	}
	limit := overviewDefaultLimit
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, overviewMaxLimit)
	}
	if c := q.Get("cursor"); c != "" {
		created, id, err := decodeRunCursor(c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		filter.BeforeCreated, filter.BeforeID = created, id
	}
	access, err := rt.resolveCallerAccess(r)
	if err != nil {
		rt.internalError(w, "api: resolve pipeline overview access failed", err)
		return
	}

	now := time.Now().UTC()
	page := pipelineRunRowsPage{Runs: []pipelineRunRowResource{}}
	more := true
	for batch := 0; batch < overviewMaxBatches && len(page.Runs) < limit && more; batch++ {
		filter.Limit = limit + 1
		rows, err := ov.ListPipelineRunRows(r.Context(), filter)
		if err != nil {
			rt.internalError(w, "api: list all pipeline runs failed", err)
			return
		}
		more = len(rows) > limit
		scan := rows
		if more {
			scan = rows[:limit]
		}
		for i, row := range scan {
			filter.BeforeCreated, filter.BeforeID = row.CreatedAt, row.ID
			if !access.can(AbilityRead, row.AppName) {
				continue
			}
			page.Runs = append(page.Runs, toRunRowResource(row, access, now))
			if len(page.Runs) == limit {
				more = more || i < len(scan)-1
				break
			}
		}
	}
	if more && filter.BeforeID != "" {
		page.NextCursor = encodeRunCursor(filter.BeforeCreated, filter.BeforeID)
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetPipelineSummary handles GET /api/v1/pipelines/summary.
func (rt *Router) handleGetPipelineSummary(w http.ResponseWriter, r *http.Request) {
	ov, ok := rt.overviewStore(w)
	if !ok {
		return
	}
	access, err := rt.resolveCallerAccess(r)
	if err != nil {
		rt.internalError(w, "api: resolve pipeline summary access failed", err)
		return
	}
	counts, err := ov.PipelineAppCounts(r.Context(), time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		rt.internalError(w, "api: pipeline summary failed", err)
		return
	}
	var sum pipelineSummaryResource
	for _, c := range counts {
		if !access.can(AbilityRead, c.AppName) {
			continue
		}
		sum.Running += c.Running
		sum.Succeeded24h += c.Succeeded24h
		sum.Failed24h += c.Failed24h
		sum.Cancelled24h += c.Cancelled24h
		sum.WaitingApproval += c.WaitingApproval
		sum.Held += c.Held
	}
	if done := sum.Succeeded24h + sum.Failed24h; done > 0 {
		rate := float64(sum.Succeeded24h) / float64(done)
		sum.SuccessRate24h = &rate
	}
	writeJSON(w, http.StatusOK, sum)
}
