package agent

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/store"
)

// SecurityStore is the persistence the security event log needs.
// *store.DB satisfies it.
type SecurityStore interface {
	AddNodeSecurityEvent(ctx context.Context, ev store.NodeSecurityEvent, p store.SecurityEventPolicy, now time.Time) error
	PruneNodeSecurityEvents(ctx context.Context, cutoff time.Time) (int64, error)
	SetNodeCAPin(ctx context.Context, nodeID, state string, now time.Time) error
	SetNodeRenewalReport(ctx context.Context, s store.NodeTrustState, now time.Time) error
}

// SecurityConfig bounds the log. Zero fields take the defaults.
type SecurityConfig struct {
	CoalesceWindow time.Duration
	PerNodeCap     int
	Retention      time.Duration
}

// Defaults for SecurityConfig.
const (
	DefaultSecurityCoalesceWindow = 10 * time.Minute
	DefaultSecurityPerNodeCap     = 500
	DefaultSecurityRetention      = 30 * 24 * time.Hour

	securityBurstGap   = 2 * time.Second
	securityMaxKeys    = 1024
	securityPruneEvery = time.Hour
	securityWriteLimit = 5 * time.Second
	securitySubjectMax = 64
	securityMessageMax = 200
)

func (c SecurityConfig) withDefaults() SecurityConfig {
	if c.CoalesceWindow <= 0 {
		c.CoalesceWindow = DefaultSecurityCoalesceWindow
	}
	if c.PerNodeCap <= 0 {
		c.PerNodeCap = DefaultSecurityPerNodeCap
	}
	if c.Retention <= 0 {
		c.Retention = DefaultSecurityRetention
	}
	return c
}

type pendingEvent struct {
	last  time.Time
	count int
	ev    store.NodeSecurityEvent
	timer *time.Timer
}

// securityRecorder writes security events without ever failing the caller.
// Coalescible events are rate limited in memory first, so a flood costs a
// bounded number of database writes however fast it arrives.
type securityRecorder struct {
	st      SecurityStore
	cfg     SecurityConfig
	logger  *slog.Logger
	now     func() time.Time
	resolve func(ctx context.Context, subject string) bool

	mu        sync.Mutex
	recent    map[string]*pendingEvent
	lastPrune time.Time
}

func newSecurityRecorder(st SecurityStore, cfg SecurityConfig, logger *slog.Logger, now func() time.Time, resolve func(context.Context, string) bool) *securityRecorder {
	return &securityRecorder{st: st, cfg: cfg.withDefaults(), logger: logger, now: now, resolve: resolve, recent: map[string]*pendingEvent{}}
}

func cleanSubject(s string) string { return cleanText(s, securitySubjectMax) }

func cleanText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > limit {
		s = string(r[:limit])
	}
	return s
}

func eventKey(ev store.NodeSecurityEvent) string {
	return strings.Join([]string{ev.NodeID, ev.Detail.Subject, ev.Detail.Message, ev.Kind, ev.Outcome, ev.Reason, ev.SourceIP}, "\x00")
}

// record stores ev. With coalesce, repeats of the same event are merged.
func (r *securityRecorder) record(ev store.NodeSecurityEvent, coalesce bool) {
	if r == nil || r.st == nil {
		return
	}
	ev.Detail.Subject = cleanSubject(ev.Detail.Subject)
	ev.Detail.Message = cleanText(ev.Detail.Message, securityMessageMax)
	if !coalesce {
		r.write(ev, false)
		return
	}
	key, now := eventKey(ev), r.now()
	r.mu.Lock()
	if p, ok := r.recent[key]; ok && now.Sub(p.last) < securityBurstGap {
		p.count++
		p.ev = ev
		if p.timer == nil {
			p.timer = time.AfterFunc(securityBurstGap, func() { r.flush(key) })
		}
		r.mu.Unlock()
		return
	}
	r.trackLocked(key, now)
	r.mu.Unlock()
	r.write(ev, true)
}

// trackLocked remembers key's last write time, evicting idle keys when
// full. A table full of live keys just stops tracking: the event is still
// written, bounded by the store cap.
func (r *securityRecorder) trackLocked(key string, now time.Time) {
	if len(r.recent) >= securityMaxKeys {
		for k, p := range r.recent {
			if p.count == 0 && now.Sub(p.last) >= securityBurstGap {
				delete(r.recent, k)
			}
		}
		if len(r.recent) >= securityMaxKeys {
			return
		}
	}
	r.recent[key] = &pendingEvent{last: now}
}

func (r *securityRecorder) flush(key string) {
	r.mu.Lock()
	p, ok := r.recent[key]
	if !ok || p.count == 0 {
		r.mu.Unlock()
		return
	}
	ev := p.ev
	ev.Count = p.count
	p.count, p.timer, p.last = 0, nil, r.now()
	r.mu.Unlock()
	r.write(ev, true)
}

func (r *securityRecorder) write(ev store.NodeSecurityEvent, coalesce bool) {
	ctx, cancel := context.WithTimeout(context.Background(), securityWriteLimit)
	defer cancel()
	if ev.NodeID == "" && ev.Detail.Subject != "" && r.resolve != nil && r.resolve(ctx, ev.Detail.Subject) {
		ev.NodeID = ev.Detail.Subject
	}
	now := r.now()
	pol := store.SecurityEventPolicy{Coalesce: coalesce, Window: r.cfg.CoalesceWindow, PerNodeCap: r.cfg.PerNodeCap}
	if err := r.st.AddNodeSecurityEvent(ctx, ev, pol, now); err != nil {
		r.logger.Warn("agent: record security event failed", slog.String("node_id", ev.NodeID), slog.String("kind", ev.Kind), slog.String("error", err.Error()))
		return
	}
	r.maybePrune(ctx, now)
}

func (r *securityRecorder) maybePrune(ctx context.Context, now time.Time) {
	r.mu.Lock()
	due := now.Sub(r.lastPrune) >= securityPruneEvery
	if due {
		r.lastPrune = now
	}
	r.mu.Unlock()
	if !due {
		return
	}
	if _, err := r.st.PruneNodeSecurityEvents(ctx, now.Add(-r.cfg.Retention)); err != nil {
		r.logger.Warn("agent: prune security events failed", slog.String("error", err.Error()))
	}
}
