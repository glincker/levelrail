package network

import (
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

// Privacy levels for the query log, applied before anything is stored.
const (
	QueryPrivacyFull         = "full"
	QueryPrivacyNamesOnly    = "names_only"
	QueryPrivacyCountersOnly = "counters_only"
)

// Reason codes explain why a query was answered the way it was.
const (
	ReasonLocalRecord = "local_record"
	ReasonNoData      = "no_data"
	ReasonNoSuchApp   = "no_such_app"
	ReasonRefused     = "refused"
	ReasonFormatError = "format_error"
)

// unknownApp keys counters for sources that map to no known app container.
const unknownApp = "unknown"

// QueryRecord is one logged query. It never carries answer data, only a count.
type QueryRecord struct {
	Time          time.Time `json:"time"`
	Source        string    `json:"source,omitempty"`
	App           string    `json:"app,omitempty"`
	QName         string    `json:"qname"`
	QType         string    `json:"qtype"`
	RCode         string    `json:"rcode"`
	Answers       int       `json:"answers"`
	LatencyMicros int64     `json:"latency_us"`
	Reason        string    `json:"reason"`
}

// AppDNSCounters are the running totals for one app.
type AppDNSCounters struct {
	App      string `json:"app"`
	Queries  uint64 `json:"queries"`
	NXDomain uint64 `json:"nxdomain"`
	Refused  uint64 `json:"refused"`
}

// QueryFilter narrows a Query call. Zero values match everything.
type QueryFilter struct {
	App    string
	Prefix string
	RCode  string
	Since  time.Time
	Limit  int
}

// QueryLogConfig supplies live settings, read on every query so a runtime
// settings change applies without a restart.
type QueryLogConfig struct {
	Enabled  func() bool
	Size     func() int
	Privacy  func() string
	AppForIP func(netip.Addr) string
}

// QueryLog is a bounded in-memory ring of query records plus per-app counters.
type QueryLog struct {
	cfg QueryLogConfig

	mu       sync.Mutex
	ring     []QueryRecord
	next     int
	full     bool
	counters map[string]*AppDNSCounters
}

// NewQueryLog builds a log that stays inert until cfg.Enabled reports true.
func NewQueryLog(cfg QueryLogConfig) *QueryLog {
	return &QueryLog{cfg: cfg, counters: map[string]*AppDNSCounters{}}
}

// Enabled reports whether queries are being recorded right now.
func (l *QueryLog) Enabled() bool {
	return l != nil && l.cfg.Enabled != nil && l.cfg.Enabled()
}

// Privacy returns the active level, falling back to names_only on anything unknown.
func (l *QueryLog) Privacy() string {
	if l == nil || l.cfg.Privacy == nil {
		return QueryPrivacyNamesOnly
	}
	switch p := l.cfg.Privacy(); p {
	case QueryPrivacyFull, QueryPrivacyNamesOnly, QueryPrivacyCountersOnly:
		return p
	}
	return QueryPrivacyNamesOnly
}

// Observe records one answered query. Privacy is applied here, before storage.
func (l *QueryLog) Observe(src netip.Addr, qname, qtype, rcode, reason string, answers int, latency time.Duration, now time.Time) {
	if !l.Enabled() {
		return
	}
	app := ""
	if l.cfg.AppForIP != nil && src.IsValid() {
		app = l.cfg.AppForIP(src)
	}
	privacy := l.Privacy()

	l.mu.Lock()
	defer l.mu.Unlock()
	key := app
	if key == "" {
		key = unknownApp
	}
	c := l.counters[key]
	if c == nil {
		c = &AppDNSCounters{App: key}
		l.counters[key] = c
	}
	c.Queries++
	switch rcode {
	case "NXDOMAIN":
		c.NXDomain++
	case "REFUSED":
		c.Refused++
	}
	if privacy == QueryPrivacyCountersOnly {
		return
	}
	rec := QueryRecord{
		Time: now.UTC(), App: app, QName: strings.ToLower(qname), QType: qtype,
		RCode: rcode, Answers: answers, LatencyMicros: latency.Microseconds(), Reason: reason,
	}
	if privacy == QueryPrivacyFull && src.IsValid() {
		rec.Source = src.String()
	}
	l.push(rec)
}

// push stores rec, resizing the ring first when the setting changed.
func (l *QueryLog) push(rec QueryRecord) {
	size := 1
	if l.cfg.Size != nil {
		size = max(l.cfg.Size(), 1)
	}
	if len(l.ring) != size {
		l.resize(size)
	}
	l.ring[l.next] = rec
	l.next = (l.next + 1) % size
	if l.next == 0 {
		l.full = true
	}
}

// resize keeps the newest records that fit the new capacity.
func (l *QueryLog) resize(size int) {
	old := l.ordered()
	if len(old) > size {
		old = old[len(old)-size:]
	}
	l.ring = make([]QueryRecord, size)
	copy(l.ring, old)
	l.next = len(old) % size
	l.full = len(old) == size
}

// ordered returns stored records oldest first. Caller holds mu.
func (l *QueryLog) ordered() []QueryRecord {
	if !l.full {
		return append([]QueryRecord(nil), l.ring[:l.next]...)
	}
	out := make([]QueryRecord, 0, len(l.ring))
	out = append(out, l.ring[l.next:]...)
	return append(out, l.ring[:l.next]...)
}

// Query returns matching records newest first, at most f.Limit when positive.
func (l *QueryLog) Query(f QueryFilter) []QueryRecord {
	if l == nil {
		return []QueryRecord{}
	}
	prefix := strings.ToLower(f.Prefix)
	l.mu.Lock()
	all := l.ordered()
	l.mu.Unlock()
	out := []QueryRecord{}
	for i := len(all) - 1; i >= 0; i-- {
		r := all[i]
		if f.App != "" && r.App != f.App {
			continue
		}
		if prefix != "" && !strings.HasPrefix(r.QName, prefix) {
			continue
		}
		if f.RCode != "" && !strings.EqualFold(r.RCode, f.RCode) {
			continue
		}
		if !f.Since.IsZero() && r.Time.Before(f.Since) {
			continue
		}
		out = append(out, r)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out
}

// Stats returns per-app counters sorted by app name, plus their sum.
func (l *QueryLog) Stats() (apps []AppDNSCounters, total AppDNSCounters) {
	apps = []AppDNSCounters{}
	total = AppDNSCounters{App: "total"}
	if l == nil {
		return apps, total
	}
	l.mu.Lock()
	for _, c := range l.counters {
		apps = append(apps, *c)
		total.Queries += c.Queries
		total.NXDomain += c.NXDomain
		total.Refused += c.Refused
	}
	l.mu.Unlock()
	sort.Slice(apps, func(i, j int) bool { return apps[i].App < apps[j].App })
	return apps, total
}
