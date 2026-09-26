package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// NoiseStore is what NoiseControl reads and writes. *DB satisfies it.
type NoiseStore interface {
	ListActiveSilences(ctx context.Context, now time.Time) ([]Silence, error)
	ListMaintenanceWindows(ctx context.Context) ([]MaintenanceWindow, error)
	RecordHistory(ctx context.Context, e HistoryEntry) error
	PruneHistory(ctx context.Context, cutoff time.Time) (int64, error)
}

// PlacementSource resolves which node an app runs on and whether it is
// offline. *store.DB satisfies it.
type PlacementSource interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	ListNodes(ctx context.Context) ([]store.Node, error)
}

// SendFunc delivers one event to its channel.
type SendFunc func(ctx context.Context, ev Event) error

// NoiseControl sits between rule evaluation and notification. It applies
// consecutive-failure thresholds, flapping detection, silences,
// maintenance windows, node-down inhibition, grouping and per-channel
// rate limits, and records every decision in alert history. State is
// in memory: a control plane restart forgets pending groups, streaks and
// suppressed fires (rule firing state itself is persisted by the engine).
type NoiseControl struct {
	cfg       NoiseConfig
	store     NoiseStore
	placement PlacementSource
	logger    *slog.Logger

	mu         sync.Mutex
	streaks    *streakTracker
	flaps      *flapTracker
	limiter    *rateLimiter
	groups     *groupBuffer
	suppressed map[string]suppression
	lastPrune  time.Time
}

type suppression struct {
	ev        Event
	outcome   string
	silenceID string
}

// NewNoiseControl builds a NoiseControl. placement may be nil, which
// disables node-based matching and inhibition.
func NewNoiseControl(cfg NoiseConfig, st NoiseStore, placement PlacementSource, logger *slog.Logger) *NoiseControl {
	if logger == nil {
		logger = slog.Default()
	}
	return &NoiseControl{
		cfg: cfg, store: st, placement: placement, logger: logger,
		streaks: newStreakTracker(), flaps: newFlapTracker(), limiter: newRateLimiter(),
		groups: newGroupBuffer(), suppressed: map[string]suppression{},
	}
}

// ApplyStreak enforces the consecutive-failure threshold on one
// evaluated rule before its state is persisted.
func (n *NoiseControl) ApplyStreak(prev, next Rule, now time.Time) Rule {
	need := prev.ConsecutiveFailures
	if need == 0 {
		need = n.cfg.ConsecutiveFailures
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.streaks.apply(prev, next, need, now)
}

// IsPlatformKind reports whether a rule kind watches the whole control
// plane rather than one app.
func IsPlatformKind(k Kind) bool {
	switch k {
	case KindCertExpiry, KindPatchStatus, KindNodeDiskSpace, KindNodeResourceUsage, KindNodeOffline,
		KindNodeCertExpiring, KindControlPlaneBackupStale, KindLogArchiveStale:
		return true
	default:
		return false
	}
}

type alertScope struct {
	ac          AlertContext
	nodeOffline bool
}

func (n *NoiseControl) describe(ctx context.Context, r Rule) alertScope {
	sc := alertScope{ac: AlertContext{RuleID: r.ID, Kind: string(r.Kind), Labels: r.Labels, Severity: severityOrDefault(r.Severity)}}
	if IsPlatformKind(r.Kind) {
		return sc
	}
	sc.ac.App = strings.TrimPrefix(r.ResourceID, "service:")
	if n.placement == nil {
		return sc
	}
	svc, err := n.placement.GetDesiredService(ctx, sc.ac.App)
	if err != nil || svc == nil || svc.NodeID == "" {
		return sc
	}
	sc.ac.NodeID = svc.NodeID
	nodes, err := n.placement.ListNodes(ctx)
	if err != nil {
		return sc
	}
	for _, node := range nodes {
		if node.ID == svc.NodeID {
			sc.ac.NodeName = node.Name
			sc.nodeOffline = node.Status == store.NodeStatusOffline
		}
	}
	return sc
}

// silenceHit is why an alert is currently muted.
type silenceHit struct {
	id     string
	detail string
}

// MuteHit names the silence or maintenance window muting an alert.
type MuteHit struct {
	ID     string
	Detail string
	Window bool
}

// FindMute reports whether any active silence or enabled maintenance
// window in effect at now mutes the alert c.
func FindMute(silences []Silence, windows []MaintenanceWindow, c AlertContext, now time.Time) (MuteHit, bool) {
	for _, s := range silences {
		if s.Status(now) == SilenceActive && s.Matchers.Matches(c) {
			return MuteHit{ID: s.ID, Detail: "silence " + s.ID}, true
		}
	}
	for _, w := range windows {
		if !w.Enabled || !w.Covers(c) {
			continue
		}
		if st, err := w.StateAt(now); err == nil && st.Active {
			return MuteHit{ID: w.ID, Detail: "maintenance window " + w.Name, Window: true}, true
		}
	}
	return MuteHit{}, false
}

func (n *NoiseControl) silencedBy(ctx context.Context, ac AlertContext, now time.Time) (silenceHit, bool) {
	silences, err := n.store.ListActiveSilences(ctx, now)
	if err != nil {
		n.logger.Error("alerting: list silences failed, treating alert as not silenced", slog.String("error", err.Error()))
	}
	windows, err := n.store.ListMaintenanceWindows(ctx)
	if err != nil {
		n.logger.Error("alerting: list maintenance windows failed, treating alert as not in maintenance", slog.String("error", err.Error()))
	}
	hit, ok := FindMute(silences, windows, ac, now)
	return silenceHit{id: hit.ID, detail: hit.Detail}, ok
}

func (n *NoiseControl) record(ctx context.Context, sc alertScope, r Rule, event, outcome, detail, silenceID string, sendErr error, now time.Time) {
	e := HistoryEntry{
		At: now, RuleID: r.ID, RuleName: r.Name, RuleKind: string(r.Kind), ResourceID: r.ResourceID,
		App: sc.ac.App, Node: firstNonEmpty(sc.ac.NodeName, sc.ac.NodeID), Severity: severityOrDefault(r.Severity),
		Event: event, Outcome: outcome, Detail: detail, SilenceID: silenceID, ChannelID: r.ChannelID,
	}
	if sendErr != nil {
		e.Error = sendErr.Error()
	}
	if err := n.store.RecordHistory(ctx, e); err != nil {
		n.logger.Error("alerting: record history failed", slog.String("rule_id", r.ID), slog.String("error", err.Error()))
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func eventName(ev Event) string {
	if ev.Resolved {
		return EventResolved
	}
	return EventFired
}

func targetKey(r Rule) string {
	if r.ChannelID != "" {
		return "channel:" + r.ChannelID
	}
	return "url:" + r.NotifyURL
}

// Route decides what happens to one firing or resolved event.
func (n *NoiseControl) Route(ctx context.Context, ev Event, now time.Time, send SendFunc) {
	r := ev.Rule
	sc := n.describe(ctx, r)

	n.mu.Lock()
	threshold, window := r.FlapThreshold, r.FlapWindow
	if threshold == 0 {
		threshold = n.cfg.FlapThreshold
	}
	if window == 0 {
		window = n.cfg.FlapWindow
	}
	flapping, entered := n.flaps.observe(ev, threshold, window, now)
	fires := n.flaps.fireCount(r.ID)
	sup, wasSuppressed := n.suppressed[r.ID]
	if ev.Resolved && wasSuppressed {
		delete(n.suppressed, r.ID)
	}
	buffered := false
	if ev.Resolved {
		buffered = n.groups.remove(r.ID)
	}
	n.mu.Unlock()

	switch {
	case ev.Resolved && wasSuppressed:
		n.record(ctx, sc, r, EventResolved, sup.outcome, "resolved; the firing notification was never sent", sup.silenceID, nil, now)
		return
	case ev.Resolved && buffered:
		n.record(ctx, sc, r, EventResolved, OutcomeGrouped, "resolved before its group was sent; nothing was delivered", "", nil, now)
		return
	case entered:
		ev.Headline = fmt.Sprintf("[FLAPPING] %s (%s) fired %d times in %s; further notifications are held until it settles", r.Name, r.Kind, fires, window)
		n.deliver(ctx, sc, ev, EventFlapping, now, send)
		return
	case flapping:
		n.record(ctx, sc, r, eventName(ev), OutcomeFlapping, "rule is flapping; notification held", "", nil, now)
		return
	}
	n.deliver(ctx, sc, ev, eventName(ev), now, send)
}

// deliver applies silences, inhibition, grouping and rate limiting, then sends.
func (n *NoiseControl) deliver(ctx context.Context, sc alertScope, ev Event, event string, now time.Time, send SendFunc) {
	r := ev.Rule
	if hit, ok := n.silencedBy(ctx, sc.ac, now); ok {
		n.holdIfFiring(ev, OutcomeSilenced, hit.id)
		n.record(ctx, sc, r, event, OutcomeSilenced, hit.detail, hit.id, nil, now)
		return
	}
	if sc.nodeOffline && !ev.Resolved && event == EventFired {
		n.holdIfFiring(ev, OutcomeInhibited, "")
		n.record(ctx, sc, r, event, OutcomeInhibited, "node "+firstNonEmpty(sc.ac.NodeName, sc.ac.NodeID)+" is offline", "", nil, now)
		return
	}
	if n.cfg.GroupWindow > 0 && event == EventFired {
		n.mu.Lock()
		n.groups.add(targetKey(r)+"|"+r.ResourceID, ev, now)
		n.mu.Unlock()
		return
	}
	n.sendNow(ctx, sc, ev, event, "", now, send)
}

func (n *NoiseControl) holdIfFiring(ev Event, outcome, silenceID string) {
	if ev.Resolved {
		return
	}
	n.mu.Lock()
	n.suppressed[ev.Rule.ID] = suppression{ev: ev, outcome: outcome, silenceID: silenceID}
	n.mu.Unlock()
}

func (n *NoiseControl) sendNow(ctx context.Context, sc alertScope, ev Event, event, detail string, now time.Time, send SendFunc) {
	r := ev.Rule
	n.mu.Lock()
	ok := n.limiter.allow(targetKey(r), n.cfg.RateLimit, n.cfg.RateWindow, now)
	n.mu.Unlock()
	if !ok {
		n.record(ctx, sc, r, event, OutcomeRateLimited, "channel rate limit reached", "", nil, now)
		return
	}
	if err := send(ctx, ev); err != nil {
		n.record(ctx, sc, r, event, OutcomeFailed, detail, "", err, now)
		return
	}
	n.record(ctx, sc, r, event, OutcomeSent, detail, "", nil, now)
}

// Recheck releases a held firing notification once its silence, window
// or node outage is over while the rule is still firing.
func (n *NoiseControl) Recheck(ctx context.Context, r Rule, now time.Time, send SendFunc) {
	n.mu.Lock()
	sup, ok := n.suppressed[r.ID]
	n.mu.Unlock()
	if !ok {
		return
	}
	sc := n.describe(ctx, r)
	if _, still := n.silencedBy(ctx, sc.ac, now); still {
		return
	}
	if sc.nodeOffline && sup.outcome == OutcomeInhibited {
		return
	}
	n.mu.Lock()
	delete(n.suppressed, r.ID)
	n.mu.Unlock()
	ev := sup.ev
	ev.Rule = r
	if n.cfg.GroupWindow > 0 {
		n.mu.Lock()
		n.groups.add(targetKey(r)+"|"+r.ResourceID, ev, now)
		n.mu.Unlock()
		return
	}
	n.sendNow(ctx, sc, ev, EventFired, "released after "+sup.outcome, now, send)
}

// Sweep flushes due groups, ends calmed flapping rules and prunes history.
// Call once per evaluation tick.
func (n *NoiseControl) Sweep(ctx context.Context, now time.Time, send SendFunc) {
	n.mu.Lock()
	due := n.groups.due(n.cfg.GroupWindow, now)
	ended := n.flaps.sweep(now)
	prune := n.cfg.HistoryRetention > 0 && now.Sub(n.lastPrune) >= time.Hour
	if prune {
		n.lastPrune = now
	}
	n.mu.Unlock()

	for _, evs := range due {
		n.flushGroup(ctx, evs, now, send)
	}
	for _, ev := range ended {
		sc := n.describe(ctx, ev.Rule)
		ev.Headline = fmt.Sprintf("[STABLE] %s (%s) stopped flapping; current state: %s", ev.Rule.Name, ev.Rule.Kind, stateWord(ev.Resolved))
		n.deliver(ctx, sc, ev, EventFlapEnded, now, send)
	}
	if prune {
		if _, err := n.store.PruneHistory(ctx, now.Add(-n.cfg.HistoryRetention)); err != nil {
			n.logger.Warn("alerting: prune history failed", slog.String("error", err.Error()))
		}
	}
}

func stateWord(resolved bool) string {
	if resolved {
		return "resolved"
	}
	return "firing"
}

// FlushAll sends every buffered group immediately, for shutdown.
func (n *NoiseControl) FlushAll(ctx context.Context, now time.Time, send SendFunc) {
	n.mu.Lock()
	due := n.groups.due(0, now)
	n.mu.Unlock()
	for _, evs := range due {
		n.flushGroup(ctx, evs, now, send)
	}
}

func (n *NoiseControl) flushGroup(ctx context.Context, evs []Event, now time.Time, send SendFunc) {
	evs = n.dropNewlyMuted(ctx, evs, now)
	if len(evs) == 0 {
		return
	}
	combined := combineGroup(evs)
	n.mu.Lock()
	allowed := n.limiter.allow(targetKey(combined.Rule), n.cfg.RateLimit, n.cfg.RateWindow, now)
	n.mu.Unlock()

	var sendErr error
	if allowed {
		sendErr = send(ctx, combined)
	}
	for _, e := range evs {
		sc := n.describe(ctx, e.Rule)
		switch {
		case !allowed:
			n.record(ctx, sc, e.Rule, EventFired, OutcomeRateLimited, "channel rate limit reached", "", nil, now)
		case sendErr != nil:
			n.record(ctx, sc, e.Rule, EventFired, OutcomeFailed, groupDetail(len(evs)), "", sendErr, now)
		case len(evs) > 1:
			n.record(ctx, sc, e.Rule, EventFired, OutcomeGrouped, groupDetail(len(evs)), "", nil, now)
		default:
			n.record(ctx, sc, e.Rule, EventFired, OutcomeSent, "", "", nil, now)
		}
	}
}

// dropNewlyMuted removes buffered events whose silence or maintenance window
// began after they were buffered, recording them as silenced and holding them.
func (n *NoiseControl) dropNewlyMuted(ctx context.Context, evs []Event, now time.Time) []Event {
	live := make([]Event, 0, len(evs))
	for _, e := range evs {
		sc := n.describe(ctx, e.Rule)
		hit, muted := n.silencedBy(ctx, sc.ac, now)
		if !muted {
			live = append(live, e)
			continue
		}
		n.holdIfFiring(e, OutcomeSilenced, hit.id)
		n.record(ctx, sc, e.Rule, EventFired, OutcomeSilenced, hit.detail, hit.id, nil, now)
	}
	return live
}

func groupDetail(count int) string {
	if count <= 1 {
		return ""
	}
	return fmt.Sprintf("sent as 1 message covering %d alerts", count)
}

// RecordSkipped notes an event dropped because its rule or channel is disabled.
func (n *NoiseControl) RecordSkipped(ctx context.Context, r Rule, resolved bool, now time.Time) {
	ev := Event{Rule: r, Resolved: resolved}
	n.record(ctx, n.describe(ctx, r), r, eventName(ev), OutcomeSkipped, "rule or channel disabled", "", nil, now)
}
