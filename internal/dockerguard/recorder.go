package dockerguard

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Decision is one request a rule flagged. It never carries a body.
type Decision struct {
	Mode       Mode
	Denied     bool
	Method     string
	Path       string
	Pattern    string
	Container  string
	Violations []Violation
	At         time.Time
}

// Rule is the primary (first) rule broken.
func (d Decision) Rule() string {
	if len(d.Violations) == 0 {
		return ""
	}
	return d.Violations[0].Rule
}

// Rules lists every distinct rule broken, primary first.
func (d Decision) Rules() []string {
	var out []string
	for _, v := range d.Violations {
		if !containsString(out, v.Rule) {
			out = append(out, v.Rule)
		}
	}
	return out
}

// Audit actions a Sink writes.
const (
	ActionDenied    = "docker_guard.denied"
	ActionWouldDeny = "docker_guard.would_deny"
	// ActionFamily matches both in an audit search.
	ActionFamily = "docker_guard"
)

// Action is the audit action name for d.
func (d Decision) Action() string {
	if d.Denied {
		return ActionDenied
	}
	return ActionWouldDeny
}

// Sink persists a decision, typically as an audit row.
type Sink interface {
	RecordDecision(ctx context.Context, d Decision) error
}

// recorder logs every decision and hands it to the sink off the request
// path. Audit-mode repeats of the same rule and endpoint are deduplicated
// per AuditDedup; enforce-mode denials are always written.
type recorder struct {
	sink    Sink
	logger  *slog.Logger
	dedup   time.Duration
	queue   chan Decision
	dropped atomic.Int64
	mu      sync.Mutex
	last    map[string]time.Time
	done    chan struct{}
	once    sync.Once
}

func newRecorder(sink Sink, t Tunables, logger *slog.Logger) *recorder {
	r := &recorder{sink: sink, logger: logger, dedup: t.AuditDedup, last: map[string]time.Time{}, done: make(chan struct{})}
	if sink == nil {
		close(r.done)
		return r
	}
	r.queue = make(chan Decision, t.RecordQueue)
	go r.run()
	return r
}

func (r *recorder) record(ctx context.Context, d Decision) {
	if d.At.IsZero() {
		d.At = time.Now()
	}
	if !d.Denied && r.duplicate(d) {
		return
	}
	r.logger.LogAttrs(ctx, slog.LevelWarn, "dockerguard: request flagged",
		slog.String("rule", d.Rule()),
		slog.String("rules", strings.Join(d.Rules(), ",")),
		slog.String("mode", string(d.Mode)),
		slog.Bool("denied", d.Denied),
		slog.String("method", d.Method),
		slog.String("path", d.Path),
		slog.String("pattern", d.Pattern),
		slog.String("container", d.Container),
	)
	if r.queue == nil {
		return
	}
	select {
	case r.queue <- d:
	default:
		r.dropped.Add(1)
	}
}

func (r *recorder) duplicate(d Decision) bool {
	key := d.Rule() + " " + d.Method + " " + d.Pattern + " " + d.Path
	if d.Pattern != "" {
		key = d.Rule() + " " + d.Method + " " + d.Pattern
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.last[key]; ok && d.At.Sub(last) < r.dedup {
		return true
	}
	r.last[key] = d.At
	return false
}

func (r *recorder) run() {
	defer close(r.done)
	for d := range r.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := r.sink.RecordDecision(ctx, d); err != nil {
			r.logger.Warn("dockerguard: record decision failed", slog.String("rule", d.Rule()), slog.String("error", err.Error()))
		}
		cancel()
	}
}

func (r *recorder) close() {
	r.once.Do(func() {
		if r.queue != nil {
			close(r.queue)
		}
	})
	<-r.done
}

// RuleCount is how often one rule fired since boot.
type RuleCount struct {
	Rule      string    `json:"rule"`
	Denied    int64     `json:"denied"`
	WouldDeny int64     `json:"would_deny"`
	LastSeen  time.Time `json:"last_seen"`
	// LastPath is the most recent canonical path, never a body.
	LastPath string `json:"last_path"`
}

// Stats counts flagged requests per rule since the guard started.
type Stats struct {
	mu    sync.Mutex
	rules map[string]*RuleCount
}

func newStats() *Stats { return &Stats{rules: map[string]*RuleCount{}} }

func (s *Stats) observe(d Decision) {
	at := d.At
	if at.IsZero() {
		at = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rule := range d.Rules() {
		c, ok := s.rules[rule]
		if !ok {
			c = &RuleCount{Rule: rule}
			s.rules[rule] = c
		}
		if d.Denied {
			c.Denied++
		} else {
			c.WouldDeny++
		}
		c.LastSeen, c.LastPath = at, d.Method+" "+d.Path
	}
}

// Snapshot returns the counters sorted by rule id.
func (s *Stats) Snapshot() []RuleCount {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RuleCount, 0, len(s.rules))
	for _, c := range s.rules {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
