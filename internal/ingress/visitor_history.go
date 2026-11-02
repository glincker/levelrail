package ingress

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultRollupTopN     = 10
	defaultRollupTrackMul = 20
	defaultFlushEvery     = time.Minute
	flushFinalTimeout     = 5 * time.Second
)

// VisitorHistoryStore is the persistence the rollups need.
type VisitorHistoryStore interface {
	UpsertVisitorRollups(ctx context.Context, rows []store.VisitorRollup) error
	ListVisitorRollups(ctx context.Context, app, domain string, since, until time.Time) ([]store.VisitorRollup, error)
	PruneVisitorRollups(ctx context.Context, cutoff time.Time) (int64, error)
}

type rollupKey struct {
	key    string
	domain string
	hour   int64
}

type rollupAcc struct {
	requests int
	status   [4]int
	visitors int
	paths    map[string]int
	refs     map[string]int
}

type seenKey struct {
	key    string
	domain string
	period int64
}

type historyCfg struct {
	st        VisitorHistoryStore
	owner     func(key string) string
	retention func() time.Duration
	logger    *slog.Logger
}

// visitorHistory holds the hour accumulators between flushes. Only counts and
// hashed visitor sets live here; no address is kept.
type visitorHistory struct {
	cfg    atomic.Pointer[historyCfg]
	on     atomic.Bool
	epoch  int64
	mu     sync.Mutex
	acc    map[rollupKey]*rollupAcc
	seen   map[seenKey]map[string]struct{}
	purged bool
}

// SetVisitorHistory turns on hourly rollups persisted to st. owner maps a route key to its
// app; retention is read at every flush, and 0 keeps nothing.
func (l *AccessLog) SetVisitorHistory(st VisitorHistoryStore, owner func(string) string, retention func() time.Duration, logger *slog.Logger) {
	if l == nil || st == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	l.hist.mu.Lock()
	if l.hist.epoch == 0 {
		l.hist.epoch = time.Now().Unix()
	}
	l.hist.mu.Unlock()
	l.hist.cfg.Store(&historyCfg{st: st, owner: owner, retention: retention, logger: logger})
	l.hist.on.Store(retention() > 0)
}

func rollupTopN() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_VISITOR_ROLLUP_TOP_N"))); err == nil && v > 0 {
		return min(v, maxVisitorTopN)
	}
	return defaultRollupTopN
}

// trackRollup folds one request into its hour. Distinct labels are capped so a
// crawler hitting random paths cannot grow memory.
func (l *AccessLog) trackRollup(key string, rec AccessRecord) {
	h := &l.hist
	period := l.saltPeriod()
	host := hostOnly(rec.Host)
	rk := rollupKey{key: key, domain: host, hour: rec.Time.Unix() / 3600}
	var hash string
	if rec.Client.IsValid() {
		hash = l.visitorHash(rec.Client.String(), rec.Time, period)
	}
	limit := rollupTopN() * defaultRollupTrackMul
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.acc == nil {
		h.acc, h.seen = map[rollupKey]*rollupAcc{}, map[seenKey]map[string]struct{}{}
	}
	a := h.acc[rk]
	if a == nil {
		a = &rollupAcc{paths: map[string]int{}, refs: map[string]int{}}
		h.acc[rk] = a
	}
	a.requests++
	if c := rec.Status / 100; c >= 2 && c <= 5 {
		a.status[c-2]++
	}
	bump(a.paths, rec.Path, limit)
	if rec.Referrer != "" {
		bump(a.refs, rec.Referrer, limit)
	}
	if hash == "" {
		return
	}
	sk := seenKey{key: key, domain: host, period: rec.Time.UnixNano() / int64(period)}
	set := h.seen[sk]
	if set == nil {
		set = map[string]struct{}{}
		h.seen[sk] = set
	}
	if _, ok := set[hash]; !ok {
		set[hash] = struct{}{}
		a.visitors++
	}
}

func bump(m map[string]int, label string, limit int) {
	if _, ok := m[label]; ok || len(m) < limit {
		m[label]++
	}
}

// FlushVisitorHistory writes the hour accumulators, drops closed hours and prunes old rows.
func (l *AccessLog) FlushVisitorHistory(ctx context.Context) error {
	if l == nil {
		return nil
	}
	cfg := l.hist.cfg.Load()
	if cfg == nil {
		return nil
	}
	h := &l.hist
	ret := cfg.retention()
	h.on.Store(ret > 0)
	if ret <= 0 {
		return l.dropHistory(ctx, cfg)
	}
	h.mu.Lock()
	h.purged = false
	now := time.Now()
	curHour := now.Unix() / 3600
	n := rollupTopN()
	rows := make([]store.VisitorRollup, 0, len(h.acc))
	for rk, a := range h.acc {
		if app := cfg.owner(rk.key); app != "" {
			rows = append(rows, store.VisitorRollup{
				App: app, Domain: rk.domain, HourStart: time.Unix(rk.hour*3600, 0).UTC(), Epoch: h.epoch,
				Requests: a.requests, Status: a.status, Visitors: a.visitors,
				TopPaths: toRollupCounts(topCounts(a.paths, n)), TopRefs: toRollupCounts(topCounts(a.refs, n)),
			})
		}
	}
	h.mu.Unlock()
	if err := cfg.st.UpsertVisitorRollups(ctx, mergeSameApp(rows)); err != nil {
		return err
	}
	h.mu.Lock()
	for rk := range h.acc {
		if rk.hour < curHour {
			delete(h.acc, rk)
		}
	}
	period := l.saltPeriod()
	curPeriod := now.UnixNano() / int64(period)
	for sk := range h.seen {
		if sk.period < curPeriod {
			delete(h.seen, sk)
		}
	}
	h.mu.Unlock()
	_, err := cfg.st.PruneVisitorRollups(ctx, now.Add(-ret))
	return err
}

// mergeSameApp folds route keys of one app that share a domain and hour, since the table keys on app.
func mergeSameApp(rows []store.VisitorRollup) []store.VisitorRollup {
	type k struct {
		app, domain string
		hour        int64
	}
	idx := map[k]int{}
	var out []store.VisitorRollup
	for _, r := range rows {
		key := k{r.App, r.Domain, r.HourStart.Unix()}
		i, ok := idx[key]
		if !ok {
			idx[key] = len(out)
			out = append(out, r)
			continue
		}
		o := &out[i]
		o.Requests += r.Requests
		o.Visitors += r.Visitors
		for j := range o.Status {
			o.Status[j] += r.Status[j]
		}
		o.TopPaths = mergeCounts(o.TopPaths, r.TopPaths)
		o.TopRefs = mergeCounts(o.TopRefs, r.TopRefs)
	}
	return out
}

func mergeCounts(a, b []store.RollupCount) []store.RollupCount {
	m := map[string]int{}
	for _, c := range a {
		m[c.Label] += c.Count
	}
	for _, c := range b {
		m[c.Label] += c.Count
	}
	return toRollupCounts(topCounts(m, rollupTopN()))
}

func toRollupCounts(in []CountedItem) []store.RollupCount {
	out := make([]store.RollupCount, len(in))
	for i, c := range in {
		out[i] = store.RollupCount{Label: c.Label, Count: c.Count}
	}
	return out
}

// dropHistory clears memory and removes stored rows once after persistence is switched off.
func (l *AccessLog) dropHistory(ctx context.Context, cfg *historyCfg) error {
	h := &l.hist
	h.mu.Lock()
	h.acc, h.seen = nil, nil
	done := h.purged
	h.purged = true
	h.mu.Unlock()
	if done {
		return nil
	}
	_, err := cfg.st.PruneVisitorRollups(ctx, time.Now().Add(100*365*24*time.Hour))
	return err
}

// RunVisitorHistory flushes every interval until ctx ends. The caller flushes once more after
// the server drains so a clean shutdown loses nothing.
func (l *AccessLog) RunVisitorHistory(ctx context.Context) {
	cfg := l.hist.cfg.Load()
	if cfg == nil {
		return
	}
	t := time.NewTicker(flushInterval())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := l.FlushVisitorHistory(ctx); err != nil && ctx.Err() == nil {
				cfg.logger.Warn("visitor history flush failed", slog.String("error", err.Error()))
			}
		}
	}
}

// FlushVisitorHistoryFinal flushes with its own short deadline for the shutdown path.
func (l *AccessLog) FlushVisitorHistoryFinal() error {
	ctx, cancel := context.WithTimeout(context.Background(), flushFinalTimeout)
	defer cancel()
	return l.FlushVisitorHistory(ctx)
}

func flushInterval() time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv("APP_VISITOR_HISTORY_FLUSH"))); err == nil && d > 0 {
		return d
	}
	return defaultFlushEvery
}

// SummarizeWithHistory merges stored rollups with the live ring so long windows survive a restart.
// A visitor seen in both parts within one salt period can be counted twice; the figures are approximate.
func (l *AccessLog) SummarizeWithHistory(ctx context.Context, app string, keys []string, o VisitorSummaryOptions) VisitorSummary {
	live := l.Summarize(keys, o)
	if l == nil || !l.hist.on.Load() {
		return live
	}
	cfg := l.hist.cfg.Load()
	if cfg == nil {
		return live
	}
	until := o.Until
	if until.IsZero() {
		until = time.Now()
	}
	cut := until
	if !live.OldestKept.IsZero() {
		cut = live.OldestKept
	}
	cutHour := cut.Truncate(time.Hour)
	first := o.Since.Truncate(time.Hour)
	if first.Before(o.Since) {
		first = first.Add(time.Hour)
	}
	domain := hostOnly(o.Domain)
	rows, err := cfg.st.ListVisitorRollups(ctx, app, domain, first, until)
	if err != nil {
		cfg.logger.Warn("visitor history read failed", slog.String("error", err.Error()))
		return live
	}
	l.hist.mu.Lock()
	epoch := l.hist.epoch
	l.hist.mu.Unlock()
	return mergeRollups(live, rows, epoch, cutHour, o.Since, until, visitorTopN(o.TopN))
}

func mergeRollups(live VisitorSummary, rows []store.VisitorRollup, epoch int64, cutHour, since, until time.Time, topN int) VisitorSummary {
	step := until.Sub(since) / visitorSeriesBuckets
	if step <= 0 {
		return live
	}
	paths, refs := map[string]int{}, map[string]int{}
	for _, c := range live.TopPaths {
		paths[c.Label] += c.Count
	}
	for _, c := range live.TopRefs {
		refs[c.Label] += c.Count
	}
	doms := map[string]*DomainVisitors{}
	for i := range live.Domains {
		d := live.Domains[i]
		doms[d.Domain] = &d
	}
	for _, r := range rows {
		// Rows of this run at or after the ring start are already in the ring.
		if r.Epoch == epoch && !r.HourStart.Before(cutHour) {
			continue
		}
		live.Requests += r.Requests
		live.Visitors += r.Visitors
		for i, k := range [4]string{"2xx", "3xx", "4xx", "5xx"} {
			live.Status[k] += r.Status[i]
		}
		for _, c := range r.TopPaths {
			paths[c.Label] += c.Count
		}
		for _, c := range r.TopRefs {
			refs[c.Label] += c.Count
		}
		d := doms[r.Domain]
		if d == nil {
			d = &DomainVisitors{Domain: r.Domain}
			doms[r.Domain] = d
		}
		d.Requests += r.Requests
		d.Visitors += r.Visitors
		if len(live.Series) == visitorSeriesBuckets {
			idx := min(max(int(r.HourStart.Sub(since)/step), 0), visitorSeriesBuckets-1)
			live.Series[idx].Requests += r.Requests
			live.Series[idx].Visitors += r.Visitors
		}
		if live.OldestKept.IsZero() || r.HourStart.Before(live.OldestKept) {
			live.OldestKept = r.HourStart
		}
	}
	live.TopPaths, live.TopRefs = topCounts(paths, topN), topCounts(refs, topN)
	live.Domains = live.Domains[:0]
	for _, d := range doms {
		live.Domains = append(live.Domains, *d)
	}
	sort.Slice(live.Domains, func(i, j int) bool {
		if live.Domains[i].Requests != live.Domains[j].Requests {
			return live.Domains[i].Requests > live.Domains[j].Requests
		}
		return live.Domains[i].Domain < live.Domains[j].Domain
	})
	return live
}
