package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainExpiryStore is what ending timed domain maintenance needs.
type DomainExpiryStore interface {
	ListExpiredDomainMaintenance(ctx context.Context, now time.Time) ([]store.DomainMaintenance, error)
	DeleteDomainMaintenance(ctx context.Context, domain string) error
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// DomainOwnerStore is optional: with it the expirer ends maintenance on a
// domain that left its app and keeps alert silences in step.
type DomainOwnerStore interface {
	ListActiveDomainMaintenance(ctx context.Context, now time.Time) ([]store.DomainMaintenance, error)
	ListServiceDomains(ctx context.Context) ([]store.ServiceDomain, error)
}

// OngoingWindowStore is optional: it resolves the status page entry of open ended maintenance.
type OngoingWindowStore interface {
	EndOngoingMaintenanceWindow(ctx context.Context, id string, at time.Time) error
}

// DomainExpirer ends a domain's maintenance once its linked window ends, its
// auto-off passes or its domain is removed. The ingress already stops serving
// the page at that instant; this removes the row, audits it, lifts the alert
// silence and asks for a prompt reconcile.
type DomainExpirer struct {
	Store    DomainExpiryStore
	Silencer *Silencer
	Nudge    func()
	Logger   *slog.Logger
	Now      func() time.Time
}

// Run ticks until ctx is done.
func (x *DomainExpirer) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return footprint.Every(ctx, "domain maintenance expiry", interval, x.Logger, x.Tick)
}

// Tick ends every expired or orphaned domain maintenance, then syncs silences.
func (x *DomainExpirer) Tick(ctx context.Context) error {
	now := time.Now().UTC()
	if x.Now != nil {
		now = x.Now().UTC()
	}
	expired, err := x.Store.ListExpiredDomainMaintenance(ctx, now)
	if err != nil {
		return fmt.Errorf("maintenance: list expired domains: %w", err)
	}
	owners, active := x.owners(ctx, now)
	ended := 0
	for _, m := range expired {
		if x.end(ctx, m, owners[m.Domain], EndReasonFor(m, now), now) {
			ended++
		}
	}
	if owners != nil {
		for _, m := range active {
			if _, owned := owners[m.Domain]; !owned && x.end(ctx, m, "", EndDomainRemoved, now) {
				ended++
			}
		}
	}
	if ended > 0 && x.Nudge != nil {
		x.Nudge()
	}
	return x.sync(ctx, owners, now)
}

// owners returns domain to app and the active rows, or nil owners when unavailable.
func (x *DomainExpirer) owners(ctx context.Context, now time.Time) (map[string]string, []store.DomainMaintenance) {
	os, ok := x.Store.(DomainOwnerStore)
	if !ok {
		return nil, nil
	}
	rows, err := os.ListServiceDomains(ctx)
	if err != nil {
		x.Logger.Warn("maintenance: list domain owners failed", slog.String("error", err.Error()))
		return nil, nil
	}
	active, err := os.ListActiveDomainMaintenance(ctx, now)
	if err != nil {
		x.Logger.Warn("maintenance: list active maintenance failed", slog.String("error", err.Error()))
		return nil, nil
	}
	apps := make(map[string]string, len(rows))
	for _, r := range rows {
		apps[r.Domain] = r.ServiceName
	}
	return apps, active
}

func (x *DomainExpirer) end(ctx context.Context, m store.DomainMaintenance, app, reason string, now time.Time) bool {
	if err := x.Store.DeleteDomainMaintenance(ctx, m.Domain); err != nil {
		x.Logger.Warn("maintenance: end domain maintenance failed", slog.String("domain", m.Domain), slog.String("error", err.Error()))
		return false
	}
	if ws, ok := x.Store.(OngoingWindowStore); ok && m.Ongoing && m.WindowID != "" {
		if err := ws.EndOngoingMaintenanceWindow(ctx, m.WindowID, now); err != nil {
			x.Logger.Warn("maintenance: resolve status entry failed", slog.String("window_id", m.WindowID), slog.String("error", err.Error()))
		}
	}
	x.audit(ctx, m, app, reason, now)
	return true
}

func (x *DomainExpirer) sync(ctx context.Context, owners map[string]string, now time.Time) error {
	os, ok := x.Store.(DomainOwnerStore)
	if x.Silencer == nil || !ok || owners == nil {
		return nil
	}
	active, err := os.ListActiveDomainMaintenance(ctx, now)
	if err != nil {
		return fmt.Errorf("maintenance: list active domains: %w", err)
	}
	if err := x.Silencer.Sync(ctx, active, owners, now); err != nil {
		return errors.Join(fmt.Errorf("maintenance: sync alert silences"), err)
	}
	return nil
}

func (x *DomainExpirer) audit(ctx context.Context, m store.DomainMaintenance, app, reason string, now time.Time) {
	e, err := NewAuditEntry(AuditActionEnded, EndedDetail(m, app, reason, SystemActor, now), SystemActor, now)
	if err != nil {
		return
	}
	if err := x.Store.SaveAuditEntry(ctx, e); err != nil {
		x.Logger.Warn("maintenance: audit domain maintenance end failed", slog.String("domain", m.Domain), slog.String("error", err.Error()))
	}
}
