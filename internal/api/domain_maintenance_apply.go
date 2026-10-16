package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ongoingWindowSpan is the placeholder end of a status entry for open ended
// maintenance; the entry is resolved when maintenance ends, long before it.
const ongoingWindowSpan = 365 * 24 * time.Hour

type ongoingWindowEnder interface {
	EndOngoingMaintenanceWindow(ctx context.Context, id string, at time.Time) error
}

func (rt *Router) silencer() *maintenance.Silencer {
	if rt.alertNoise == nil {
		return nil
	}
	return &maintenance.Silencer{Store: rt.alertNoise}
}

// maintenanceFailure explains why enabling maintenance changed nothing.
type maintenanceFailure struct {
	Status int
	Domain string
	Stage  string
	Err    error
}

func (f *maintenanceFailure) Error() string {
	return fmt.Sprintf("%s for %s: %v", f.Stage, f.Domain, f.Err)
}

// maintenancePlan is one domain's prepared change.
type maintenancePlan struct {
	row       store.DomainMaintenance
	prev      store.DomainMaintenance
	silenceID string
	newWindow string
}

// enableMaintenance turns maintenance on for every domain of app, all or
// none: any failure before the rows are saved undoes the silences and status
// entries already created and names the domain and stage that failed.
func (rt *Router) enableMaintenance(ctx context.Context, app string, domains []string, req domainMaintenanceRequest, actor maintenance.Actor) ([]store.DomainMaintenance, *maintenanceFailure) {
	ss, ok := rt.domainMaintenanceSettings()
	if !ok {
		return nil, &maintenanceFailure{Status: http.StatusNotImplemented, Stage: "settings", Err: errors.New("maintenance settings are not available")}
	}
	now := time.Now().UTC()
	in := maintenance.DomainInput{
		Title: req.Title, Message: req.Message, RetryAfterSeconds: req.RetryAfterSeconds,
		Until: req.Until, Bypass: req.Bypass, BypassHealthCheck: req.BypassHealthCheck == nil || *req.BypassHealthCheck,
	}
	var requested *time.Duration
	if req.MaxDurationSeconds != nil {
		d := time.Duration(*req.MaxDurationSeconds) * time.Second
		requested = &d
	}
	autoOff, err := maintenance.ResolveAutoOff(now, req.Until, requested, envDuration(envMaintenanceMaxDuration, 0))
	if err != nil {
		return nil, &maintenanceFailure{Status: http.StatusBadRequest, Stage: "validate", Err: err}
	}
	if req.Until != nil && rt.scheduledMaintenance == nil {
		return nil, &maintenanceFailure{Status: http.StatusNotImplemented, Stage: "validate", Err: errors.New("an end time needs scheduled maintenance, which is not enabled on this server")}
	}
	plans := make([]*maintenancePlan, 0, len(domains))
	for _, d := range domains {
		m, err := in.Validate(d, now)
		if err != nil {
			return nil, &maintenanceFailure{Status: http.StatusBadRequest, Domain: d, Stage: "validate", Err: err}
		}
		prev, wasOn, err := ss.GetDomainMaintenanceSettings(ctx, d, now)
		if err != nil {
			return nil, &maintenanceFailure{Status: http.StatusInternalServerError, Domain: d, Stage: "load", Err: err}
		}
		m.StartedAt, m.AutoOffAt = maintenance.KeepStart(prev, wasOn, now), autoOff
		plans = append(plans, &maintenancePlan{row: m, prev: prev})
	}
	if f := rt.prepareMaintenance(ctx, app, plans, req.Until, now); f != nil {
		rt.undoMaintenance(ctx, plans, now)
		return nil, f
	}
	rows := make([]store.DomainMaintenance, len(plans))
	for i, p := range plans {
		rows[i] = p.row
	}
	if err := rt.saveMaintenanceRows(ctx, ss, rows); err != nil {
		rt.undoMaintenance(ctx, plans, now)
		return nil, &maintenanceFailure{Status: http.StatusInternalServerError, Domain: rows[0].Domain, Stage: "save", Err: err}
	}
	sil := rt.silencer()
	for _, p := range plans {
		if sil != nil && p.silenceID != "" {
			if err := sil.Settle(ctx, p.row.Domain, p.silenceID, now); err != nil {
				rt.logger.Warn("api: lift superseded maintenance silence failed", slog.String("domain", p.row.Domain), slog.String("error", err.Error()))
			}
		}
		if p.prev.WindowID != "" && p.prev.WindowID != p.row.WindowID {
			rt.dropMaintenanceWindow(ctx, p.prev.WindowID)
		}
		rt.auditMaintenance(ctx, maintenance.AuditActionStarted, maintenance.StartedDetail(p.row, app, actor), actor, now)
	}
	rt.nudgeReconciler()
	return rows, nil
}

func (rt *Router) saveMaintenanceRows(ctx context.Context, ss DomainMaintenanceSettingsStore, rows []store.DomainMaintenance) error {
	if all, ok := ss.(interface {
		SaveDomainMaintenanceAll(ctx context.Context, rows []store.DomainMaintenance) error
	}); ok {
		return all.SaveDomainMaintenanceAll(ctx, rows)
	}
	for _, m := range rows {
		if err := ss.SaveDomainMaintenance(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// prepareMaintenance creates each domain's silence and status entry.
func (rt *Router) prepareMaintenance(ctx context.Context, app string, plans []*maintenancePlan, until *time.Time, now time.Time) *maintenanceFailure {
	sil := rt.silencer()
	for _, p := range plans {
		d := p.row.Domain
		if sil != nil {
			p.row.EndsAt = time.Time{}
			if until != nil {
				p.row.EndsAt = until.UTC()
			}
			id, err := sil.Open(ctx, p.row, app, now)
			if err != nil {
				return &maintenanceFailure{Status: http.StatusInternalServerError, Domain: d, Stage: "silence", Err: err}
			}
			p.silenceID = id
		}
		if rt.scheduledMaintenance == nil {
			continue
		}
		win := maintenance.DomainWindow(p.row, app, now, now.Add(ongoingWindowSpan))
		if until != nil {
			win.EndsAt = until.UTC()
		} else {
			win.Ongoing = true
		}
		created, err := rt.scheduledMaintenance.InsertMaintenanceWindow(ctx, win)
		if err != nil {
			return &maintenanceFailure{Status: http.StatusInternalServerError, Domain: d, Stage: "status_entry", Err: err}
		}
		p.newWindow, p.row.WindowID = created.ID, created.ID
		if until != nil {
			p.row.EndsAt = created.EndsAt
		} else {
			p.row.EndsAt, p.row.Ongoing = time.Time{}, true
		}
	}
	return nil
}

// undoMaintenance removes what prepareMaintenance created, best effort: a
// leftover silence without a row is lifted by the next expiry pass.
func (rt *Router) undoMaintenance(ctx context.Context, plans []*maintenancePlan, now time.Time) {
	sil := rt.silencer()
	for _, p := range plans {
		if sil != nil && p.silenceID != "" {
			if err := sil.Discard(ctx, p.silenceID, now); err != nil {
				rt.logger.Warn("api: undo maintenance silence failed", slog.String("domain", p.row.Domain), slog.String("error", err.Error()))
			}
		}
		rt.dropMaintenanceWindow(ctx, p.newWindow)
	}
}

// clearMaintenance turns maintenance off for domain. Idempotent: a domain
// that was never in maintenance only has stray silences lifted.
func (rt *Router) clearMaintenance(ctx context.Context, app, domain string, actor maintenance.Actor) error {
	now := time.Now().UTC()
	var prev store.DomainMaintenance
	wasOn := false
	if ss, ok := rt.domainMaintenanceSettings(); ok {
		m, on, err := ss.GetDomainMaintenanceSettings(ctx, domain, now)
		if err != nil {
			return fmt.Errorf("load: %w", err)
		}
		prev, wasOn = m, on
	}
	if err := rt.domainMaintenance.DeleteDomainMaintenance(ctx, domain); err != nil {
		return fmt.Errorf("clear: %w", err)
	}
	if sil := rt.silencer(); sil != nil {
		if err := sil.Settle(ctx, domain, "", now); err != nil {
			rt.logger.Warn("api: lift maintenance silence failed", slog.String("domain", domain), slog.String("error", err.Error()))
		}
	}
	rt.finishMaintenanceWindow(ctx, prev, now)
	if wasOn {
		prev.Domain = domain
		rt.auditMaintenance(ctx, maintenance.AuditActionEnded, maintenance.EndedDetail(prev, app, maintenance.EndManual, actor, now), actor, now)
	}
	rt.nudgeReconciler()
	return nil
}

// finishMaintenanceWindow resolves an ongoing status entry or cancels a dated one.
func (rt *Router) finishMaintenanceWindow(ctx context.Context, m store.DomainMaintenance, now time.Time) {
	if m.WindowID == "" || rt.scheduledMaintenance == nil {
		return
	}
	if e, ok := rt.scheduledMaintenance.(ongoingWindowEnder); ok && m.Ongoing {
		if err := e.EndOngoingMaintenanceWindow(ctx, m.WindowID, now); err != nil {
			rt.logger.Warn("api: resolve maintenance status entry failed", slog.String("window_id", m.WindowID), slog.String("error", err.Error()))
		}
		return
	}
	rt.dropMaintenanceWindow(ctx, m.WindowID)
}

func (rt *Router) auditMaintenance(ctx context.Context, action string, d maintenance.AuditDetail, actor maintenance.Actor, now time.Time) {
	e, err := maintenance.NewAuditEntry(action, d, actor, now)
	if err != nil {
		rt.logger.Warn("api: build maintenance audit failed", slog.String("error", err.Error()))
		return
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, e); err != nil {
		rt.logger.Warn("api: save maintenance audit failed", slog.String("domain", d.Domain), slog.String("error", err.Error()))
	}
}

// maintenanceActor describes the caller for audit rows.
func (rt *Router) maintenanceActor(r *http.Request) maintenance.Actor {
	a := maintenance.Actor{Type: "token", ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), RemoteAddr: clientIP(r)}
	if _, ok := rt.currentSessionUserID(r); ok {
		a.Type = "session"
	}
	if _, id, name, ok := rt.currentActor(r); ok {
		a.ID, a.Name = id, name
	}
	return a
}
