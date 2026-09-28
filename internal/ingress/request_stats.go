package ingress

import (
	"errors"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func init() {
	caddy.RegisterModule(requestStatsModule{})
}

// RequestStatsHandler is the wire shape of the request_stats handler
// (http.handlers.request_stats): it times the rest of the route and records
// status class, latency bucket and byte counts under Key, the route's first
// configured host. Paths, query strings and client addresses are never read.
type RequestStatsHandler struct {
	Handler string `json:"handler"`
	Key     string `json:"key"`
}

// NewRequestStatsHandler builds the handler recording under key.
func NewRequestStatsHandler(key string) RequestStatsHandler {
	return RequestStatsHandler{Handler: "request_stats", Key: key}
}

type requestStatsModule struct {
	Key string `json:"key,omitempty"`
}

func (requestStatsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.request_stats",
		New: func() caddy.Module { return new(requestStatsModule) },
	}
}

func (m requestStatsModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	start := time.Now()
	rec := &statusRecorder{ResponseWriterWrapper: &caddyhttp.ResponseWriterWrapper{ResponseWriter: w}}
	err := next.ServeHTTP(rec, r)

	status := rec.status
	upstreamErr := false
	if err != nil {
		var herr caddyhttp.HandlerError
		code := http.StatusInternalServerError
		if errors.As(err, &herr) && herr.StatusCode > 0 {
			code = herr.StatusCode
		}
		if status == 0 {
			status = code
		}
		upstreamErr = code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
	}
	if status == 0 {
		status = http.StatusOK
	}
	in := r.ContentLength
	if in < 0 {
		in = 0
	}
	DefaultRequestStats().Observe(m.Key, status, time.Since(start), in, rec.bytes, upstreamErr)
	return err
}

type statusRecorder struct {
	*caddyhttp.ResponseWriterWrapper
	status int
	bytes  int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 && (code >= 200 || code == http.StatusSwitchingProtocols) {
		s.status = code
	}
	s.ResponseWriterWrapper.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriterWrapper.Write(p)
	s.bytes += int64(n)
	return n, err
}

func (s *statusRecorder) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{s}, r)
}

var (
	_ caddyhttp.MiddlewareHandler = requestStatsModule{}
	_ caddy.Module                = requestStatsModule{}
)

type hostCounters struct {
	mu sync.Mutex
	w  telemetry.RequestWindow
}

// RequestStats aggregates request activity per route key in process memory.
// Memory is bounded by the number of configured hosts: one fixed-size
// counter set per key, never per path or client.
type RequestStats struct {
	mu     sync.RWMutex
	hosts  map[string]*hostCounters
	owners atomic.Pointer[map[string]string]
}

var defaultRequestStats = &RequestStats{hosts: make(map[string]*hostCounters)}

// DefaultRequestStats is the process-wide aggregator the request_stats
// handler feeds; Caddy modules are constructed from JSON and cannot receive
// a live value any other way.
func DefaultRequestStats() *RequestStats { return defaultRequestStats }

// NewRequestStats returns an empty aggregator.
func NewRequestStats() *RequestStats {
	return &RequestStats{hosts: make(map[string]*hostCounters)}
}

func latencyBucket(d time.Duration) int {
	ms := float64(d) / float64(time.Millisecond)
	i := sort.SearchFloat64s(telemetry.LatencyBucketsMS, ms)
	return i
}

// Observe records one finished request.
func (s *RequestStats) Observe(key string, status int, d time.Duration, bytesIn, bytesOut int64, upstreamErr bool) {
	s.mu.RLock()
	h := s.hosts[key]
	s.mu.RUnlock()
	if h == nil {
		s.mu.Lock()
		if h = s.hosts[key]; h == nil {
			h = &hostCounters{}
			s.hosts[key] = h
		}
		s.mu.Unlock()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.w.Requests++
	switch {
	case status >= 500:
		h.w.Status5xx++
	case status >= 400:
		h.w.Status4xx++
	case status >= 300:
		h.w.Status3xx++
	default:
		h.w.Status2xx++
	}
	if upstreamErr {
		h.w.UpstreamErrors++
	}
	if bytesIn > 0 {
		h.w.BytesIn += uint64(bytesIn)
	}
	if bytesOut > 0 {
		h.w.BytesOut += uint64(bytesOut)
	}
	h.w.Latency[latencyBucket(d)]++
}

// SetHostOwners replaces the host-to-app map used to attribute traffic. Keys
// are configured route hosts, values are app names.
func (s *RequestStats) SetHostOwners(owners map[string]string) {
	cp := make(map[string]string, len(owners))
	for k, v := range owners {
		cp[k] = v
	}
	s.owners.Store(&cp)
}

// DrainByApp returns and resets activity since the last call, merged per
// app. Keys with no owner are dropped.
func (s *RequestStats) DrainByApp() map[string]telemetry.RequestWindow {
	s.mu.Lock()
	drained := make(map[string]telemetry.RequestWindow, len(s.hosts))
	for key, h := range s.hosts {
		h.mu.Lock()
		w := h.w
		h.w = telemetry.RequestWindow{}
		h.mu.Unlock()
		if w.Requests > 0 {
			drained[key] = w
		}
	}
	s.mu.Unlock()

	out := make(map[string]telemetry.RequestWindow)
	ownersPtr := s.owners.Load()
	if ownersPtr == nil {
		return out
	}
	for key, w := range drained {
		app, ok := (*ownersPtr)[key]
		if !ok {
			continue
		}
		merged := out[app]
		merged.Add(w)
		out[app] = merged
	}
	return out
}
