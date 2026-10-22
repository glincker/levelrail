package ingress

import (
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/kit/ids"
)

const (
	headerRequestID   = "X-Request-Id"
	headerTraceparent = "Traceparent"
	// defaultAccessCapacity is the per-route-key ring size when
	// APP_ACCESS_LOG_SIZE is unset; 0 turns the log off.
	defaultAccessCapacity = 1000
)

// AccessRecord is one finished edge request. The query string and client
// address are never captured.
type AccessRecord struct {
	Time      time.Time `json:"time"`
	Host      string    `json:"host"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Millis    float64   `json:"duration_ms"`
	BytesIn   int64     `json:"bytes_in"`
	BytesOut  int64     `json:"bytes_out"`
	RequestID string    `json:"request_id"`
	TraceID   string    `json:"trace_id,omitempty"`
	Upstream  bool      `json:"upstream_error,omitempty"`
	Cold      bool      `json:"cold_start,omitempty"`
}

// AccessFilter narrows a ring read.
type AccessFilter struct {
	MinMillis float64
	MinStatus int
	Since     time.Time
	Path      string
	Limit     int
}

type accessRing struct {
	mu   sync.Mutex
	buf  []AccessRecord
	next int
	full bool
}

func (r *accessRing) add(rec AccessRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = rec
	r.next++
	if r.next == len(r.buf) {
		r.next, r.full = 0, true
	}
}

func (r *accessRing) snapshot() []AccessRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]AccessRecord(nil), r.buf[:r.next]...)
	}
	out := make([]AccessRecord, 0, len(r.buf))
	out = append(out, r.buf[r.next:]...)
	return append(out, r.buf[:r.next]...)
}

// AccessLog keeps the most recent requests per route key in process memory.
// Memory is bounded by hosts times capacity and the log is lost on restart,
// which is the point: it answers "what just happened", not history.
type AccessLog struct {
	mu        sync.RWMutex
	cap       int
	retention time.Duration
	rings     map[string]*accessRing

	subs    atomic.Pointer[[]accessSub]
	subMu   sync.Mutex
	nextSub uint64
}

type accessSub struct {
	id uint64
	fn func(key string, rec AccessRecord)
}

// Subscribe registers fn to see every recorded request and returns a func
// that removes it. fn runs on the request path, so it must never block.
func (l *AccessLog) Subscribe(fn func(key string, rec AccessRecord)) (unsubscribe func()) {
	if l == nil || fn == nil {
		return func() {}
	}
	l.subMu.Lock()
	defer l.subMu.Unlock()
	l.nextSub++
	id := l.nextSub
	var next []accessSub
	if p := l.subs.Load(); p != nil {
		next = append(next, *p...)
	}
	next = append(next, accessSub{id: id, fn: fn})
	l.subs.Store(&next)
	return func() {
		l.subMu.Lock()
		defer l.subMu.Unlock()
		var kept []accessSub
		if p := l.subs.Load(); p != nil {
			for _, s := range *p {
				if s.id != id {
					kept = append(kept, s)
				}
			}
		}
		if len(kept) == 0 {
			l.subs.Store(nil)
			return
		}
		l.subs.Store(&kept)
	}
}

func (l *AccessLog) notify(key string, rec AccessRecord) {
	p := l.subs.Load()
	if p == nil {
		return
	}
	for _, s := range *p {
		s.fn(key, rec)
	}
}

var defaultAccessLog = newAccessLogWithRetention(accessCapacityFromEnv(), accessRetentionFromEnv())

// DefaultAccessLog is the process-wide log the request_stats handler feeds.
func DefaultAccessLog() *AccessLog { return defaultAccessLog }

// NewAccessLog returns a log holding capacity records per key; capacity <= 0
// disables recording.
func NewAccessLog(capacity int) *AccessLog {
	return &AccessLog{cap: capacity, rings: make(map[string]*accessRing)}
}

func newAccessLogWithRetention(capacity int, retention time.Duration) *AccessLog {
	l := NewAccessLog(capacity)
	l.retention = retention
	return l
}

// accessRetentionFromEnv reads APP_ACCESS_LOG_RETENTION; zero keeps records until the ring overwrites them.
func accessRetentionFromEnv() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv("APP_ACCESS_LOG_RETENTION")))
	if err != nil || d < 0 {
		return 0
	}
	return d
}

func accessCapacityFromEnv() int {
	v := strings.TrimSpace(os.Getenv("APP_ACCESS_LOG_SIZE"))
	if v == "" {
		return defaultAccessCapacity
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultAccessCapacity
	}
	return n
}

// Record stores one request under key.
func (l *AccessLog) Record(key string, rec AccessRecord) {
	if l == nil {
		return
	}
	if l.cap <= 0 {
		l.notify(key, rec)
		return
	}
	l.mu.RLock()
	r := l.rings[key]
	l.mu.RUnlock()
	if r == nil {
		l.mu.Lock()
		if r = l.rings[key]; r == nil {
			r = &accessRing{buf: make([]AccessRecord, l.cap)}
			l.rings[key] = r
		}
		l.mu.Unlock()
	}
	r.add(rec)
	l.notify(key, rec)
}

// Query returns matching records for the given keys, newest first.
func (l *AccessLog) Query(keys []string, f AccessFilter) []AccessRecord {
	if l == nil {
		return nil
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []AccessRecord
	for _, key := range keys {
		l.mu.RLock()
		r := l.rings[key]
		l.mu.RUnlock()
		if r == nil {
			continue
		}
		for _, rec := range r.snapshot() {
			if l.retention > 0 && time.Since(rec.Time) > l.retention {
				continue
			}
			if rec.Millis < f.MinMillis || rec.Status < f.MinStatus {
				continue
			}
			if !f.Since.IsZero() && rec.Time.Before(f.Since) {
				continue
			}
			if f.Path != "" && !strings.HasPrefix(rec.Path, f.Path) {
				continue
			}
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// KeysForApp returns the route keys owned by app.
func (s *RequestStats) KeysForApp(app string) []string {
	p := s.owners.Load()
	if p == nil {
		return nil
	}
	var keys []string
	for k, v := range *p {
		if v == app {
			keys = append(keys, k)
		}
	}
	return keys
}

// ensureCorrelationHeaders gives the request an X-Request-Id and a W3C
// traceparent when the client sent none, so the app and its own OTLP spans
// can be matched to the edge record. It returns the ids to record.
func ensureCorrelationHeaders(r *http.Request) (requestID, traceID string) {
	requestID = r.Header.Get(headerRequestID)
	if requestID == "" || len(requestID) > 128 {
		requestID = randHex(8)
		r.Header.Set(headerRequestID, requestID)
	}
	if tid, ok := parseTraceparent(r.Header.Get(headerTraceparent)); ok {
		return requestID, tid
	}
	traceID = randHex(16)
	r.Header.Set(headerTraceparent, "00-"+traceID+"-"+randHex(8)+"-01")
	return requestID, traceID
}

func parseTraceparent(v string) (string, bool) {
	parts := strings.Split(v, "-")
	if len(parts) != 4 || len(parts[0]) != 2 || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return "", false
	}
	if parts[1] == strings.Repeat("0", 32) || !isLowerHex(parts[1]) || !isLowerHex(parts[2]) {
		return "", false
	}
	return parts[1], true
}

func isLowerHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func randHex(n int) string {
	return ids.MustHex(n)
}
