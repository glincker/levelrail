package incident

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	linkAuthor     = "system"
	autoLabel      = "[Automated] "
	statusResolved = "resolved"
)

// Audit actions for system written incident rows.
const (
	// AuditLinkOpened is written when a monitor outage opens a status page incident.
	AuditLinkOpened = "monitor_incident.opened"
	// AuditLinkResolved is written when that incident resolves.
	AuditLinkResolved = "monitor_incident.resolved"
)

// LinkStore is the persistence the Linker needs.
type LinkStore interface {
	GetMonitor(ctx context.Context, id string) (monitor.Monitor, error)
	ListMonitors(ctx context.Context) ([]monitor.Monitor, error)
	ListStatusComponents(ctx context.Context) ([]store.StatusComponent, error)
	ListStatusIncidents(ctx context.Context, limit int) ([]store.StatusIncident, error)
	ListOpenLinkedIncidents(ctx context.Context) ([]store.InternalStatusIncident, error)
	CreateInternalStatusIncident(ctx context.Context, in store.InternalStatusIncident) error
	SaveStatusIncident(ctx context.Context, i store.StatusIncident) error
	SetStatusIncidentInternalState(ctx context.Context, id, state, by string, at time.Time) error
	AddInternalIncidentUpdate(ctx context.Context, u store.StatusIncidentUpdate, visibility, author string) error
}

// Auditor records one system audit row.
type Auditor func(ctx context.Context, action, monitorID, incidentID string)

// Publisher queues a status webhook for a public incident; opened is false on resolve.
type Publisher func(ctx context.Context, incidentID string, opened bool)

// Linker opens a public status page incident for an opted-in monitor that is confirmed down and
// resolves it after the recovery period. Paging stays with the monitor_down alert rule.
type Linker struct {
	cfg     LinkConfig
	store   LinkStore
	guard   Guard
	mons    MonitorState
	audit   Auditor
	publish Publisher
	logger  *slog.Logger
	now     func() time.Time
	queue   chan monitor.Transition

	mu      sync.Mutex
	upSince map[string]time.Time
}

// NewLinker builds a Linker. guard and audit may be nil; a nil guard means nothing is muted.
func NewLinker(cfg LinkConfig, st LinkStore, g Guard, mons MonitorState, audit Auditor, logger *slog.Logger) *Linker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Linker{cfg: cfg, store: st, guard: g, mons: mons, audit: audit, logger: logger, now: time.Now,
		queue: make(chan monitor.Transition, max(cfg.QueueMax, 1)), upSince: map[string]time.Time{}}
}

// SetPublisher wires the status webhook emitter from the composition root, so this package
// does not import the API or alerting layers.
func (l *Linker) SetPublisher(p Publisher) {
	l.mu.Lock()
	l.publish = p
	l.mu.Unlock()
}

func (l *Linker) emit(ctx context.Context, incidentID string, opened bool) {
	l.mu.Lock()
	p := l.publish
	l.mu.Unlock()
	if p != nil {
		p(ctx, incidentID, opened)
	}
}

// SetMonitors wires the monitor service once it exists (it needs Notify first).
func (l *Linker) SetMonitors(m MonitorState) {
	l.mu.Lock()
	l.mons = m
	l.mu.Unlock()
}

// Notify is a monitor.Notifier hook that never blocks the probe worker; the sweep catches drops up.
func (l *Linker) Notify(t monitor.Transition) {
	select {
	case l.queue <- t:
	default:
		l.logger.Warn("monitor link queue full, dropping transition", slog.String("monitor", t.Monitor.ID))
	}
}

// Run drains transitions and sweeps until ctx ends.
func (l *Linker) Run(ctx context.Context) {
	tick := time.NewTicker(l.cfg.SweepInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-l.queue:
			l.Reconcile(ctx, t.Monitor.ID)
		case <-tick.C:
			l.Sweep(ctx)
		}
	}
}

// Sweep re-evaluates every monitor, which applies the confirmation and recovery periods over time
// and heals a restart that lost the in-memory tracker.
func (l *Linker) Sweep(ctx context.Context) {
	list, err := l.store.ListMonitors(ctx)
	if err != nil {
		l.logger.Error("monitor link sweep: list monitors failed", slog.String("error", err.Error()))
		return
	}
	live := map[string]bool{}
	for _, m := range list {
		live[m.ID] = true
		l.reconcile(ctx, m)
	}
	open, err := l.store.ListOpenLinkedIncidents(ctx)
	if err != nil {
		return
	}
	for _, inc := range open {
		if !live[inc.SourceRef] {
			l.resolve(ctx, inc, "monitor deleted")
		}
	}
}

// Reconcile applies the state machine to one monitor by id.
func (l *Linker) Reconcile(ctx context.Context, id string) {
	m, err := l.store.GetMonitor(ctx, id)
	if err != nil {
		return
	}
	l.reconcile(ctx, m)
}

func (l *Linker) reconcile(ctx context.Context, m monitor.Monitor) {
	open, err := l.store.ListOpenLinkedIncidents(ctx)
	if err != nil {
		l.logger.Error("monitor link: list open incidents failed", slog.String("error", err.Error()))
		return
	}
	var cur *store.InternalStatusIncident
	for i := range open {
		if open[i].SourceRef == m.ID {
			cur = &open[i]
		}
	}
	l.mu.Lock()
	mons := l.mons
	l.mu.Unlock()
	if mons == nil {
		return
	}
	st := mons.Status(m.ID)
	switch {
	case cur != nil && !m.Spec.StatusLink:
		l.resolve(ctx, *cur, "status link turned off")
	case !m.Spec.StatusLink:
	case st.State == monitor.StateDown:
		l.clearUp(m.ID)
		if cur == nil {
			l.open(ctx, m, st)
		}
	case st.State == monitor.StateUp && st.FailStreak == 0:
		if cur != nil && l.recovered(m) {
			l.resolve(ctx, *cur, "monitor recovered")
		}
	default:
		// suspect or recovering: any wobble restarts the recovery clock, which is the flap guard.
		l.clearUp(m.ID)
	}
}

func (l *Linker) clearUp(id string) {
	l.mu.Lock()
	delete(l.upSince, id)
	l.mu.Unlock()
}

func (l *Linker) recovered(m monitor.Monitor) bool {
	period := l.cfg.RecoveryPeriod
	if s := m.Spec.LinkRecoverySeconds; s > 0 {
		period = time.Duration(s) * time.Second
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	since, ok := l.upSince[m.ID]
	if !ok {
		since = now
		l.upSince[m.ID] = now
	}
	if now.Sub(since) < period {
		return false
	}
	delete(l.upSince, m.ID)
	return true
}

func (l *Linker) open(ctx context.Context, m monitor.Monitor, st monitor.Status) {
	need := l.cfg.ConfirmFailures
	if n := m.Spec.LinkConfirmFailures; n > 0 {
		need = n
	}
	if st.FailStreak < need {
		return
	}
	now := l.now()
	if l.guard != nil {
		if detail, muted := l.guard.Muted(ctx, m.ID, m.Name, now); muted {
			l.logger.Info("monitor link suppressed by mute", slog.String("monitor", m.ID), slog.String("mute", detail))
			return
		}
	}
	comps, err := l.store.ListStatusComponents(ctx)
	if err != nil {
		l.logger.Error("monitor link: list components failed", slog.String("error", err.Error()))
		return
	}
	var ids []string
	for _, c := range comps {
		if c.Kind == componentKind && c.Target == m.ID {
			ids = append(ids, c.ID)
		}
	}
	if l.manualOpenFor(ctx, ids) {
		l.logger.Info("monitor link skipped, an operator incident is already open", slog.String("monitor", m.ID))
		return
	}
	in := store.InternalStatusIncident{
		StatusIncident: store.StatusIncident{ID: newID("inc_"), Kind: incidentKind, Title: m.Name + " is down", Status: statusOpen,
			Impact: l.cfg.Impact, ComponentIDs: ids, StartsAt: now},
		StatusIncidentInternal: store.StatusIncidentInternal{Source: store.SourceMonitorStatus, SourceRef: m.ID,
			InternalState: store.IncidentStateTriggered, Published: true},
	}
	if err := l.store.CreateInternalStatusIncident(ctx, in); err != nil {
		// The partial unique index turns a concurrent open into an error, which is the idempotency guard.
		l.logger.Warn("monitor link: open incident failed", slog.String("monitor", m.ID), slog.String("error", err.Error()))
		return
	}
	l.update(ctx, in.ID, statusOpen, autoLabel+m.Name+" failed its health checks and was confirmed down.", now)
	l.record(ctx, AuditLinkOpened, m.ID, in.ID)
	l.emit(ctx, in.ID, true)
}

// manualOpenFor reports an unresolved operator incident already covering one of the components.
func (l *Linker) manualOpenFor(ctx context.Context, ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	list, err := l.store.ListStatusIncidents(ctx, 100)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(list, func(i store.StatusIncident) bool {
		return i.Status != statusResolved && slices.ContainsFunc(i.ComponentIDs, func(c string) bool { return slices.Contains(ids, c) })
	})
}

func (l *Linker) resolve(ctx context.Context, inc store.InternalStatusIncident, why string) {
	now := l.now()
	pub := inc.StatusIncident
	pub.Status, pub.ResolvedAt, pub.EndsAt = statusResolved, &now, &now
	if err := l.store.SaveStatusIncident(ctx, pub); err != nil {
		l.logger.Error("monitor link: resolve failed", slog.String("incident", inc.ID), slog.String("error", err.Error()))
		return
	}
	if err := l.store.SetStatusIncidentInternalState(ctx, inc.ID, store.IncidentStateResolved, linkAuthor, now); err != nil {
		l.logger.Error("monitor link: resolve state failed", slog.String("incident", inc.ID), slog.String("error", err.Error()))
		return
	}
	l.update(ctx, inc.ID, statusResolved, fmt.Sprintf("%sResolved: %s.", autoLabel, why), now)
	l.record(ctx, AuditLinkResolved, inc.SourceRef, inc.ID)
	l.emit(ctx, inc.ID, false)
}

func (l *Linker) update(ctx context.Context, incidentID, status, body string, at time.Time) {
	u := store.StatusIncidentUpdate{ID: newID("iu_"), IncidentID: incidentID, Status: status, Body: body, CreatedAt: at}
	if err := l.store.AddInternalIncidentUpdate(ctx, u, store.UpdateVisibilityPublic, linkAuthor); err != nil {
		l.logger.Error("monitor link: timeline entry failed", slog.String("incident", incidentID), slog.String("error", err.Error()))
	}
}

func (l *Linker) record(ctx context.Context, action, monitorID, incidentID string) {
	if l.audit != nil {
		l.audit(ctx, action, monitorID, incidentID)
	}
}

// NewAuditID returns a random id for a system audit row.
func NewAuditID() string { return newID("aud_") }
