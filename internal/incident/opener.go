package incident

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	sourceMonitor  = "monitor"
	overflowRef    = "overflow"
	systemAuthor   = "system"
	componentKind  = "monitor"
	incidentKind   = "incident"
	statusOpen     = "investigating"
	impactDefault  = "major"
	updateResolved = "resolved"
)

// Store is the persistence the opener needs.
type Store interface {
	ListStatusComponents(ctx context.Context) ([]store.StatusComponent, error)
	ListMonitorIncidents(ctx context.Context, since time.Time) ([]store.InternalStatusIncident, error)
	ListStatusIncidentLinks(ctx context.Context, incidentID string) ([]store.StatusIncidentLink, error)
	CreateInternalStatusIncident(ctx context.Context, in store.InternalStatusIncident) error
	SetStatusIncidentInternalState(ctx context.Context, id, state, by string, at time.Time) error
	AddInternalIncidentUpdate(ctx context.Context, u store.StatusIncidentUpdate, visibility, author string) error
	AddStatusIncidentLink(ctx context.Context, incidentID, monitorID string, downAt time.Time) error
	MarkStatusIncidentLinkUp(ctx context.Context, incidentID, monitorID string, upAt time.Time) error
}

// Guard answers the alerting questions: is monitor_down enabled, and is this outage muted.
type Guard interface {
	RuleEnabled(ctx context.Context) bool
	Muted(ctx context.Context, monitorID, name string, now time.Time) (detail string, muted bool)
}

// MonitorState is the live monitor view the sweep reconciles against.
type MonitorState interface {
	List(ctx context.Context) ([]monitor.Monitor, error)
	Status(id string) monitor.Status
}

// Opener turns confirmed monitor transitions into internal, unpublished incidents.
type Opener struct {
	cfg    Config
	store  Store
	guard  Guard
	logger *slog.Logger
	now    func() time.Time
	queue  chan monitor.Transition

	mu       sync.Mutex
	monitors MonitorState
}

// New builds an Opener. The queue keeps Notify non-blocking for the probe worker.
func New(cfg Config, st Store, g Guard, logger *slog.Logger) *Opener {
	if logger == nil {
		logger = slog.Default()
	}
	return &Opener{cfg: cfg, store: st, guard: g, logger: logger, now: time.Now, queue: make(chan monitor.Transition, max(cfg.QueueMax, 1))}
}

// SetMonitors wires the monitor service once it exists (it needs Notify first).
func (o *Opener) SetMonitors(m MonitorState) {
	o.mu.Lock()
	o.monitors = m
	o.mu.Unlock()
}

// Notify is the monitor.Notifier hook. On overflow it drops and logs; the sweep catches up.
func (o *Opener) Notify(t monitor.Transition) {
	select {
	case o.queue <- t:
	default:
		o.logger.Warn("incident queue full, dropping transition", slog.String("monitor", t.Monitor.ID))
	}
}

// Run drains transitions and runs the sweep until ctx ends.
func (o *Opener) Run(ctx context.Context) {
	tick := time.NewTicker(o.cfg.SweepInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-o.queue:
			o.Handle(ctx, t)
		case <-tick.C:
			o.Sweep(ctx)
		}
	}
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// Handle applies one confirmed transition.
func (o *Opener) Handle(ctx context.Context, t monitor.Transition) {
	var err error
	switch t.State {
	case monitor.StateDown:
		err = o.handleDown(ctx, t)
	case monitor.StateUp:
		err = o.handleUp(ctx, t.Monitor.ID)
	}
	if err != nil {
		o.logger.Error("incident handling failed", slog.String("monitor", t.Monitor.ID), slog.String("error", err.Error()))
	}
}

func (o *Opener) note(ctx context.Context, incidentID, body string) error {
	u := store.StatusIncidentUpdate{ID: newID("iu_"), IncidentID: incidentID, Status: statusOpen, Body: body, CreatedAt: o.now()}
	return o.store.AddInternalIncidentUpdate(ctx, u, store.UpdateVisibilityInternal, systemAuthor)
}

// foldKeys are the component ids plus their group ids. A component follows one monitor, so
// monitors "share a component" only through the group they sit in on the status page.
func foldKeys(all []store.StatusComponent, ids []string) []string {
	var keys []string
	for _, c := range all {
		if slices.Contains(ids, c.ID) {
			keys = append(keys, "c:"+c.ID)
			if c.GroupID != "" {
				keys = append(keys, "g:"+c.GroupID)
			}
		}
	}
	return keys
}

func monitorComponentIDs(all []store.StatusComponent, monitorID string) []string {
	var ids []string
	for _, c := range all {
		if c.Kind == componentKind && c.Target == monitorID {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

func (o *Opener) handleDown(ctx context.Context, t monitor.Transition) error {
	now := o.now()
	m := t.Monitor
	if !o.guard.RuleEnabled(ctx) {
		return nil
	}
	if detail, muted := o.guard.Muted(ctx, m.ID, m.Name, now); muted {
		o.logger.Info("incident suppressed by mute", slog.String("monitor", m.ID), slog.String("mute", detail))
		return nil
	}
	since := now.Add(-max(o.cfg.ReopenWindow, o.cfg.FoldWindow))
	incs, err := o.store.ListMonitorIncidents(ctx, since)
	if err != nil {
		return err
	}
	linked := map[string][]store.StatusIncidentLink{}
	for _, inc := range incs {
		if linked[inc.ID], err = o.store.ListStatusIncidentLinks(ctx, inc.ID); err != nil {
			return err
		}
	}
	reason := fmt.Sprintf("Monitor %s confirmed down: %s", m.Name, t.Reason)

	// Already tracked by an open incident: nothing to do.
	for _, inc := range incs {
		if inc.InternalState != store.IncidentStateResolved && hasOpenLink(linked[inc.ID], m.ID) {
			return nil
		}
	}
	// Same monitor went down again while its incident stayed open for other monitors.
	for _, inc := range incs {
		if inc.InternalState != store.IncidentStateResolved && (inc.SourceRef == m.ID || hasLink(linked[inc.ID], m.ID)) {
			return o.attach(ctx, inc.ID, m.ID, now, reason)
		}
	}
	// Reopen a recently resolved incident of this monitor.
	for _, inc := range incs {
		if inc.InternalState == store.IncidentStateResolved && inc.InternalResolvedAt != nil &&
			!inc.InternalResolvedAt.Before(now.Add(-o.cfg.ReopenWindow)) && (inc.SourceRef == m.ID || hasLink(linked[inc.ID], m.ID)) {
			if err := o.store.SetStatusIncidentInternalState(ctx, inc.ID, store.IncidentStateTriggered, systemAuthor, now); err != nil {
				return err
			}
			return o.attach(ctx, inc.ID, m.ID, now, "Reopened. "+reason)
		}
	}
	all, err := o.store.ListStatusComponents(ctx)
	if err != nil {
		return err
	}
	comps := monitorComponentIDs(all, m.ID)
	myKeys := foldKeys(all, comps)
	open := 0
	for _, inc := range incs {
		if inc.InternalState != store.IncidentStateResolved {
			open++
			if overlaps(foldKeys(all, inc.ComponentIDs), myKeys) && lastDown(linked[inc.ID]).After(now.Add(-o.cfg.FoldWindow)) {
				return o.attach(ctx, inc.ID, m.ID, now, "Folded in. "+reason)
			}
		}
	}
	if open >= o.cfg.OpenMax {
		o.logger.Warn("open incident cap reached, folding into overflow incident", slog.Int("cap", o.cfg.OpenMax))
		for _, inc := range incs {
			if inc.InternalState != store.IncidentStateResolved && inc.SourceRef == overflowRef {
				return o.attach(ctx, inc.ID, m.ID, now, reason)
			}
		}
		return o.create(ctx, overflowRef, "Multiple monitors are down", m.ID, nil, now, reason)
	}
	return o.create(ctx, m.ID, "Monitor "+m.Name+" is down", m.ID, comps, now, reason)
}

func (o *Opener) create(ctx context.Context, ref, title, monitorID string, comps []string, now time.Time, body string) error {
	in := store.InternalStatusIncident{
		StatusIncident: store.StatusIncident{ID: newID("inc_"), Kind: incidentKind, Title: title, Status: statusOpen, Impact: impactDefault,
			ComponentIDs: comps, StartsAt: now},
		StatusIncidentInternal: store.StatusIncidentInternal{Source: sourceMonitor, SourceRef: ref, InternalState: store.IncidentStateTriggered, Published: false},
	}
	if err := o.store.CreateInternalStatusIncident(ctx, in); err != nil {
		return err
	}
	return o.attach(ctx, in.ID, monitorID, now, body)
}

func (o *Opener) attach(ctx context.Context, incidentID, monitorID string, now time.Time, body string) error {
	if err := o.store.AddStatusIncidentLink(ctx, incidentID, monitorID, now); err != nil {
		return err
	}
	return o.note(ctx, incidentID, body)
}

// handleUp closes the monitor's link and resolves the incident once every link is up.
func (o *Opener) handleUp(ctx context.Context, monitorID string) error {
	return o.closeLink(ctx, monitorID, "monitor recovered")
}

func (o *Opener) closeLink(ctx context.Context, monitorID, why string) error {
	now := o.now()
	incs, err := o.store.ListMonitorIncidents(ctx, now)
	if err != nil {
		return err
	}
	for _, inc := range incs {
		if inc.InternalState == store.IncidentStateResolved {
			continue
		}
		links, err := o.store.ListStatusIncidentLinks(ctx, inc.ID)
		if err != nil {
			return err
		}
		if !hasOpenLink(links, monitorID) {
			continue
		}
		if err := o.store.MarkStatusIncidentLinkUp(ctx, inc.ID, monitorID, now); err != nil {
			return err
		}
		if err := o.note(ctx, inc.ID, "Monitor "+monitorID+": "+why); err != nil {
			return err
		}
		stillDown := false
		for _, l := range links {
			if l.UpAt == nil && l.MonitorID != monitorID {
				stillDown = true
			}
		}
		if stillDown {
			continue
		}
		if err := o.store.SetStatusIncidentInternalState(ctx, inc.ID, store.IncidentStateResolved, systemAuthor, now); err != nil {
			return err
		}
		u := store.StatusIncidentUpdate{ID: newID("iu_"), IncidentID: inc.ID, Status: updateResolved, Body: "All linked monitors recovered", CreatedAt: now}
		if err := o.store.AddInternalIncidentUpdate(ctx, u, store.UpdateVisibilityInternal, systemAuthor); err != nil {
			return err
		}
	}
	return nil
}

// Sweep reconciles open incidents with live monitor state: it covers a restart during an outage
// (the tracker starts clear and fires no up) and monitors deleted while linked.
func (o *Opener) Sweep(ctx context.Context) {
	o.mu.Lock()
	mons := o.monitors
	o.mu.Unlock()
	if mons == nil {
		return
	}
	list, err := mons.List(ctx)
	if err != nil {
		o.logger.Error("incident sweep: list monitors failed", slog.String("error", err.Error()))
		return
	}
	exists := map[string]bool{}
	for _, m := range list {
		exists[m.ID] = true
	}
	incs, err := o.store.ListMonitorIncidents(ctx, o.now())
	if err != nil {
		o.logger.Error("incident sweep: list incidents failed", slog.String("error", err.Error()))
		return
	}
	for _, inc := range incs {
		if inc.InternalState == store.IncidentStateResolved {
			continue
		}
		links, err := o.store.ListStatusIncidentLinks(ctx, inc.ID)
		if err != nil {
			o.logger.Error("incident sweep: list links failed", slog.String("error", err.Error()))
			return
		}
		for _, l := range links {
			if l.UpAt != nil {
				continue
			}
			why := ""
			switch st := mons.Status(l.MonitorID); {
			case !exists[l.MonitorID]:
				why = "monitor deleted"
			case st.State == monitor.StateUp && st.FailStreak == 0:
				why = "monitor recovered (sweep)"
			}
			if why == "" {
				continue
			}
			if err := o.closeLink(ctx, l.MonitorID, why); err != nil {
				o.logger.Error("incident sweep: close failed", slog.String("monitor", l.MonitorID), slog.String("error", err.Error()))
			}
		}
	}
}

func hasOpenLink(links []store.StatusIncidentLink, monitorID string) bool {
	return slices.ContainsFunc(links, func(l store.StatusIncidentLink) bool { return l.MonitorID == monitorID && l.UpAt == nil })
}

func hasLink(links []store.StatusIncidentLink, monitorID string) bool {
	return slices.ContainsFunc(links, func(l store.StatusIncidentLink) bool { return l.MonitorID == monitorID })
}

func overlaps(a, b []string) bool {
	return slices.ContainsFunc(a, func(x string) bool { return slices.Contains(b, x) })
}

// lastDown is the newest down time among the links, so an incident that is still churning stays foldable.
func lastDown(links []store.StatusIncidentLink) time.Time {
	var last time.Time
	for _, l := range links {
		if l.DownAt.After(last) {
			last = l.DownAt
		}
	}
	return last
}
