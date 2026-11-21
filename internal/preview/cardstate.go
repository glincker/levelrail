package preview

import (
	"context"
	"fmt"
	"log/slog"
)

// CardState is what a list card needs to draw: a render state, a short
// reason when the render failed, and the best record to show meanwhile.
type CardState struct {
	State  string
	Reason string
	Record *Record
}

func isScreenshot(r Record) bool {
	return r.HasFile() && (r.Source == "" || r.Source == SourceScreenshot)
}

// CardStates reports the card state of each app in apps in one store read.
// An app on the screenshot tier with no screenshot and no remembered failure
// gets one background render queued: the call never waits for a browser.
func (m *Manager) CardStates(ctx context.Context, apps []string) (map[string]CardState, error) {
	recs, err := m.deps.Store.ListAllPreviewRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("preview: list latest records: %w", err)
	}
	want := make(map[string]bool, len(apps))
	for _, a := range apps {
		want[a] = true
	}
	shot := map[string]Record{}
	latest := map[string]Record{}
	for _, r := range recs {
		if !want[r.App] || r.Status != StatusOK || (!r.HasFile() && r.Meta == "") {
			continue
		}
		if cur, ok := latest[r.App]; !ok || r.CapturedAt.After(cur.CapturedAt) {
			latest[r.App] = r
		}
		if cur, ok := shot[r.App]; isScreenshot(r) && (!ok || r.CapturedAt.After(cur.CapturedAt)) {
			shot[r.App] = r
		}
	}
	out := make(map[string]CardState, len(apps))
	for _, app := range apps {
		out[app] = m.cardState(ctx, app, shot, latest)
	}
	return out, nil
}

func (m *Manager) cardState(ctx context.Context, app string, shot, anyRec map[string]Record) CardState {
	st := CardState{State: StateNone}
	if r, ok := anyRec[app]; ok {
		st.Record = &r
	}
	s, err := m.Settings(ctx, app)
	if err != nil || !m.cfg.Enabled || s.Mode == ModeOff {
		return st
	}
	if r, ok := shot[app]; ok {
		st.State, st.Record = StateReady, &r
		m.refreshStale(app)
		return st
	}
	if s.Mode == ModeMetadata {
		if st.Record != nil {
			st.State = StateReady
		}
		return st
	}
	switch {
	case m.isActive(app):
		st.State = StateRendering
	case m.isQueued(app):
		st.State = StateQueued
	default:
		if reason, _ := m.deps.Store.GetPreviewState(ctx, failKeyPrefix+app); reason != "" {
			st.State, st.Reason = StateFailed, reason
		} else if m.intervalOK(app) {
			st.State = StateQueued
			m.enqueue(job{app: app, auto: true})
		}
	}
	return st
}

// refreshStale queues the render a rate-limited deploy skipped, once the
// minimum interval has passed.
func (m *Manager) refreshStale(app string) {
	m.mu.Lock()
	stale := m.stale[app]
	m.mu.Unlock()
	if stale && m.intervalOK(app) {
		m.log.Debug("preview: rendering after rate limit", slog.String("app", app))
		m.enqueue(job{app: app, auto: true})
	}
}

func (m *Manager) isActive(app string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active[app]
}

func (m *Manager) isQueued(app string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.queue {
		if j.app == app {
			return true
		}
	}
	return false
}
