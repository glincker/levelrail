package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/ticker"
)

// DomainExpiryStore is what ending timed domain maintenance needs.
type DomainExpiryStore interface {
	ListExpiredDomainMaintenance(ctx context.Context, now time.Time) ([]store.DomainMaintenance, error)
	DeleteDomainMaintenance(ctx context.Context, domain string) error
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// DomainExpirer ends a domain's maintenance once its linked window ends or
// is cancelled. The ingress already stops serving the page at that instant;
// this removes the row, audits it and asks for a prompt reconcile.
type DomainExpirer struct {
	Store  DomainExpiryStore
	Nudge  func()
	Logger *slog.Logger
	Now    func() time.Time
}

// Run ticks until ctx is done.
func (x *DomainExpirer) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return ticker.Run(ctx, interval, x.Logger, "domain maintenance expiry", x.Tick)
}

// Tick ends every expired domain maintenance.
func (x *DomainExpirer) Tick(ctx context.Context) error {
	now := time.Now().UTC()
	if x.Now != nil {
		now = x.Now().UTC()
	}
	expired, err := x.Store.ListExpiredDomainMaintenance(ctx, now)
	if err != nil {
		return fmt.Errorf("maintenance: list expired domains: %w", err)
	}
	ended := 0
	for _, m := range expired {
		if err := x.Store.DeleteDomainMaintenance(ctx, m.Domain); err != nil {
			x.Logger.Warn("maintenance: end domain maintenance failed", slog.String("domain", m.Domain), slog.String("error", err.Error()))
			continue
		}
		ended++
		x.audit(ctx, m, now)
	}
	if ended > 0 && x.Nudge != nil {
		x.Nudge()
	}
	return nil
}

func (x *DomainExpirer) audit(ctx context.Context, m store.DomainMaintenance, now time.Time) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	e := store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "maintenance-expiry", ActorName: "Scheduled maintenance",
		Ability: store.AuditActionDomainMaintenanceEnded, Method: "EVENT",
		Path:       "/api/v1/domains/" + m.Domain + "/maintenance",
		StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(now), ClientKind: "system",
		Action: store.AuditActionDomainMaintenanceEnded,
	}
	if err := x.Store.SaveAuditEntry(ctx, e); err != nil {
		x.Logger.Warn("maintenance: audit domain maintenance end failed", slog.String("domain", m.Domain), slog.String("error", err.Error()))
	}
}
