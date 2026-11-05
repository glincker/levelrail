package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	parkedRetentionReason    = "retention"
	parkedRetentionActorID   = "parked-retention"
	parkedRetentionActorName = "Parked domain retention"
	// parkedRetentionBatch bounds one run so a large backlog drains over several hours.
	parkedRetentionBatch    = 50
	envParkedRetentionEvery = "APP_PARKED_RETENTION_INTERVAL"
	defaultParkedRetention  = time.Hour
)

// parkedRetentionState remembers the last run so the dry run can say when the next one is due.
type parkedRetentionState struct{ lastUnix atomic.Int64 }

type parkedRetentionCandidate struct {
	Domain   string    `json:"domain"`
	ParkedAt time.Time `json:"parked_at"`
	AgeDays  int       `json:"age_days"`
}

type parkedRetentionPlan struct {
	RetentionDays int                        `json:"retention_days"`
	WouldDelete   []parkedRetentionCandidate `json:"would_delete"`
	TotalEligible int                        `json:"total_eligible"`
	BatchLimit    int                        `json:"batch_limit"`
	NextRun       *time.Time                 `json:"next_run,omitempty"`
}

// planParkedRetention picks what one run deletes: oldest first, bounded, nothing at 0 days.
// The job and the dry run share it so the preview is exactly the job's selection.
func planParkedRetention(rows []store.ParkedDomain, days int, now time.Time) parkedRetentionPlan {
	plan := parkedRetentionPlan{RetentionDays: days, WouldDelete: []parkedRetentionCandidate{}, BatchLimit: parkedRetentionBatch}
	if days <= 0 {
		return plan
	}
	cutoff := now.Add(-time.Duration(days) * hoursPerDay * time.Hour)
	var due []parkedRetentionCandidate
	for _, p := range rows {
		if p.ParkedAt.IsZero() || !p.ParkedAt.Before(cutoff) {
			continue
		}
		due = append(due, parkedRetentionCandidate{Domain: p.Domain, ParkedAt: p.ParkedAt.UTC(), AgeDays: int(now.Sub(p.ParkedAt) / (hoursPerDay * time.Hour))})
	}
	sort.Slice(due, func(i, j int) bool {
		if !due[i].ParkedAt.Equal(due[j].ParkedAt) {
			return due[i].ParkedAt.Before(due[j].ParkedAt)
		}
		return due[i].Domain < due[j].Domain
	})
	plan.TotalEligible = len(due)
	if len(due) > parkedRetentionBatch {
		due = due[:parkedRetentionBatch]
	}
	plan.WouldDelete = append(plan.WouldDelete, due...)
	return plan
}

func (rt *Router) parkedRetentionInterval() time.Duration {
	return envDuration(envParkedRetentionEvery, defaultParkedRetention)
}

// handleParkedRetention handles GET /api/v1/domains/parked/retention, a dry run
// that deletes nothing.
func (rt *Router) handleParkedRetention(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.requireParkedStore(w)
	if !ok {
		return
	}
	rows, err := st.ListParkedDomains(r.Context())
	if err != nil {
		rt.internalError(w, "api: parked retention dry run", err)
		return
	}
	now := time.Now()
	plan := planParkedRetention(rows, rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDomainsParkedRetentionDays), now)
	if plan.RetentionDays > 0 {
		next := now.Add(rt.parkedRetentionInterval())
		if last := rt.parkedRetention.lastUnix.Load(); last > 0 {
			next = time.Unix(last, 0).Add(rt.parkedRetentionInterval())
		}
		next = next.UTC()
		plan.NextRun = &next
	}
	writeJSON(w, http.StatusOK, plan)
}

// SweepParkedRetention deletes the parked registrations the plan picks and
// audits each. It never touches DNS or certificates, and is a no-op at 0 days.
func (rt *Router) SweepParkedRetention(ctx context.Context, now time.Time) (int, error) {
	rt.parkedRetention.lastUnix.Store(now.Unix())
	days := rt.runtimeSettings.Int(ctx, runtimesettings.KeyDomainsParkedRetentionDays)
	if days <= 0 {
		return 0, nil
	}
	st := rt.parkedStore()
	if st == nil {
		return 0, nil
	}
	rows, err := st.ListParkedDomains(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, c := range planParkedRetention(rows, days, now).WouldDelete {
		if err := st.DeleteParkedDomain(ctx, c.Domain); err != nil {
			if errors.Is(err, store.ErrParkedDomainNotFound) {
				continue
			}
			return deleted, err
		}
		deleted++
		rt.recordParkedRetentionAudit(ctx, c, now)
	}
	return deleted, nil
}

func (rt *Router) recordParkedRetentionAudit(ctx context.Context, c parkedRetentionCandidate, at time.Time) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: parked retention audit id", slog.String("domain", c.Domain), slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]any{"reason": parkedRetentionReason, "age_days": c.AgeDays})
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSystem, ActorID: parkedRetentionActorID, ActorName: parkedRetentionActorName,
		Ability: AbilityWriteSensitive, Method: auditMethodEvent, Path: "/api/v1/domains/parked/" + c.Domain, StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(at), ClientKind: auditClientKindSystem, Action: auditActionParkedDeleted, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save parked retention audit entry", slog.String("domain", c.Domain), slog.String("error", err.Error()))
	}
}

// RunParkedRetentionSweeper runs SweepParkedRetention on the configured interval until ctx ends.
func (rt *Router) RunParkedRetentionSweeper(ctx context.Context) error {
	return footprint.Every(ctx, "parked domain retention", rt.parkedRetentionInterval(), rt.logger, func(ctx context.Context) error {
		n, err := rt.SweepParkedRetention(ctx, time.Now())
		if err == nil && n > 0 {
			rt.logger.Info("api: deleted expired parked domains", slog.Int("deleted", n))
		}
		return err
	})
}
