package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionJobGuardsChange is the audit event for a job guard rail change.
// Its detail lists changed key names only, never values.
const AuditActionJobGuardsChange = "job.guards.change"

// jobGuardStore is the optional store surface for guard rails; *store.DB has it.
type jobGuardStore interface {
	GetScheduledTaskGuards(ctx context.Context, id string) (store.ScheduledTaskGuards, error)
	SetScheduledTaskGuards(ctx context.Context, id string, g store.ScheduledTaskGuards) error
}

// overlapPolicyOf maps the stored concurrency policy onto the overlap enum
// (allow, skip, queue-one); replace and queue have no overlap name.
func overlapPolicyOf(policy string) string {
	switch policy {
	case store.ScheduledTaskConcurrencyAllow, "":
		return store.ScheduledTaskConcurrencyAllow
	case store.ScheduledTaskConcurrencyForbid:
		return store.ScheduledTaskConcurrencySkip
	case store.ScheduledTaskConcurrencyQueueOne:
		return store.ScheduledTaskConcurrencyQueueOne
	}
	return ""
}

func (rt *Router) withJobGuards(ctx context.Context, res scheduledTaskResource) scheduledTaskResource {
	res.OverlapPolicy = overlapPolicyOf(res.ConcurrencyPolicy)
	gs, ok := rt.scheduledTasks.(jobGuardStore)
	if !ok || res.ID == "" {
		return res
	}
	if g, err := gs.GetScheduledTaskGuards(ctx, res.ID); err == nil {
		res.AlertOnFailure, res.KeepRuns, res.KeepDays = g.AlertOnFailure, g.KeepRuns, g.KeepDays
	}
	return res
}

func guardsOf(res scheduledTaskResource) store.ScheduledTaskGuards {
	return store.ScheduledTaskGuards{AlertOnFailure: res.AlertOnFailure, KeepRuns: res.KeepRuns, KeepDays: res.KeepDays}
}

// persistJobGuards saves the guard rails and audits which guard keys
// changed against prev (the job before this request).
func (rt *Router) persistJobGuards(r *http.Request, app string, prev, next store.ScheduledTask, prevGuards store.ScheduledTaskGuards, req scheduledTaskResource) error {
	gs, ok := rt.scheduledTasks.(jobGuardStore)
	if !ok {
		return nil
	}
	g := guardsOf(req)
	var changed []string
	if prev.ConcurrencyPolicy != next.ConcurrencyPolicy {
		changed = append(changed, "overlap_policy")
	}
	if prev.TimeoutSeconds != next.TimeoutSeconds {
		changed = append(changed, "timeout_seconds")
	}
	// The config keys are already saved, so they are audited even if the guard write fails.
	if g != prevGuards {
		if err := gs.SetScheduledTaskGuards(r.Context(), next.ID, g); err != nil {
			if len(changed) > 0 {
				rt.auditJobGuards(r, app, next.ID, changed)
			}
			return err
		}
	}
	if prevGuards.AlertOnFailure != g.AlertOnFailure {
		changed = append(changed, "alert_on_failure")
	}
	if prevGuards.KeepRuns != g.KeepRuns {
		changed = append(changed, "keep_runs")
	}
	if prevGuards.KeepDays != g.KeepDays {
		changed = append(changed, "keep_days")
	}
	if len(changed) > 0 {
		rt.auditJobGuards(r, app, next.ID, changed)
	}
	return nil
}

func (rt *Router) auditJobGuards(r *http.Request, app, id string, changed []string) {
	if rt.auditLog == nil {
		rt.logger.Warn("api: job guards changed with no audit log configured", slog.String("app", app))
		return
	}
	a := rt.accessActor(r)
	entryID, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: job guards audit id failed", slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]any{"app": app, "job_id": id, "changed": changed})
	entry := store.AuditEntry{
		ID: entryID, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionJobGuardsChange,
		Detail:     string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: job guards audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}
