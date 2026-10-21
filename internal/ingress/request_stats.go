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
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func init() {
	caddy.RegisterModule(requestStatsModule{})
}

// RequestStatsHandler is the wire shape of the request_stats handler
// (http.handlers.request_stats): it times the rest of the route and records
// status class, latency bucket and byte counts under Key, the route's first
// configured host. Query strings and client addresses are never read.
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
	requestID, traceID := ensureCorrelationHeaders(r)
	w.Header().Set(headerRequestID, requestID)
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
	elapsed := time.Since(start)
	DefaultRequestStats().ObserveRequest(m.Key, RequestObservation{
		Method: r.Method, Path: r.URL.Path, Status: status, Duration: elapsed, BytesIn: in, BytesOut: rec.bytes,
		UpstreamErr: upstreamErr, Upstream: upstreamHostport(r), RequestID: requestID, At: start,
		Referrer: requestReferrerHost(r), Country: requestCountry(r),
	})
	DefaultAccessLog().Record(m.Key, AccessRecord{
		Time: start, Host: r.Host, Method: r.Method, Path: r.URL.Path, Status: status,
		Millis: float64(elapsed) / float64(time.Millisecond), BytesIn: in, BytesOut: rec.bytes,
		RequestID: requestID, TraceID: traceID, Upstream: upstreamErr, Cold: rec.cold,
	})
	return err
}

type statusRecorder struct {
	*caddyhttp.ResponseWriterWrapper
	status int
	bytes  int64
	cold   bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if h := s.Header(); h.Get(WakeHeldHeader) != "" {
		s.cold = true
		h.Del(WakeHeldHeader)
	}
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

	totalsMu sync.Mutex
	totals   telemetry.RequestWindow
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
	s.ObservePath(key, "", status, d, bytesIn, bytesOut, upstreamErr)
}

var (
	routesEnabled = telemetry.RoutesEnabled()
	routeMax      = telemetry.RouteMaxFromEnv()
	routeDepth    = telemetry.RouteDepthFromEnv()
	upstreamMax   = telemetry.UpstreamMaxFromEnv()
	audienceMax   = telemetry.AudienceMaxFromEnv()
	sampleCfg     = telemetry.SampleConfigFromEnv()
)

// ObservePath records one finished request and, when route counting is on and
// path is non-empty, its normalized route template. Only the path is read,
// never the query string, and ids are collapsed before anything is stored.
func (s *RequestStats) ObservePath(key, path string, status int, d time.Duration, bytesIn, bytesOut int64, upstreamErr bool) {
	s.ObserveRequest(key, RequestObservation{Path: path, Status: status, Duration: d, BytesIn: bytesIn, BytesOut: bytesOut, UpstreamErr: upstreamErr})
}

// RequestObservation is one finished request as the stats see it. Upstream is
// the replica host:port; no query string or client address is ever included.
type RequestObservation struct {
	Method      string
	Path        string
	Status      int
	Duration    time.Duration
	BytesIn     int64
	BytesOut    int64
	UpstreamErr bool
	Upstream    string
	RequestID   string
	At          time.Time
	// Referrer is the Referer header's host only; Country an ISO code.
	Referrer string
	Country  string
}

// ObserveRequest records o under key: counters, route, replica histogram and,
// for slow or failed requests, a bounded sample.
func (s *RequestStats) ObserveRequest(key string, o RequestObservation) {
	if o.At.IsZero() {
		o.At = time.Now()
	}
	status, d, path := o.Status, o.Duration, o.Path
	bytesIn, bytesOut, upstreamErr := o.BytesIn, o.BytesOut, o.UpstreamErr
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
	if o.Upstream != "" {
		s.totalsMu.Lock()
		s.totals.ObserveUpstream(o.Upstream, status, upstreamErr, float64(d)/float64(time.Millisecond), upstreamMax)
		s.totalsMu.Unlock()
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
	route := ""
	if routesEnabled && path != "" {
		route = telemetry.NormalizeRoute(path, routeDepth)
	}
	ms := float64(d) / float64(time.Millisecond)
	h.w.ObserveRoute(route, status, ms, routeMax)
	h.w.ObserveUpstream(o.Upstream, status, upstreamErr, ms, upstreamMax)
	h.w.ObserveAudience(o.Referrer, o.Country, audienceMax)
	if path != "" && sampleCfg.ShouldSample(status, ms, upstreamErr, o.RequestID) {
		h.w.AddSample(telemetry.RequestSample{
			Time: o.At, Route: telemetry.NormalizeRoute(path, routeDepth), Method: o.Method, Status: status,
			DurationMS: ms, Upstream: o.Upstream, RequestID: o.RequestID,
		}, sampleCfg.TickMax)
	}
}

// UpstreamTotals returns a copy of the cumulative per-replica counters since
// process start. Unlike DrainByApp it never resets, so a caller can diff two
// snapshots to judge one time window (the canary reconciler does).
func (s *RequestStats) UpstreamTotals() map[string]telemetry.UpstreamStat {
	s.totalsMu.Lock()
	defer s.totalsMu.Unlock()
	out := make(map[string]telemetry.UpstreamStat, len(s.totals.Upstreams))
	for k, v := range s.totals.Upstreams {
		out[k] = v
	}
	return out
}

// ObserveShield counts one request the abuse shield turned away under key.
func (s *RequestStats) ObserveShield(key string, reason trafficpolicy.ShieldReason) {
	if reason < 0 || reason >= trafficpolicy.ShieldReasonCount {
		return
	}
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
	h.w.Shield[reason]++
	h.mu.Unlock()
}

// upstreamHostport is the replica Caddy's reverse proxy picked for r, empty
// when the request never reached an upstream.
func upstreamHostport(r *http.Request) string {
	repl, ok := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
	if !ok || repl == nil {
		return ""
	}
	v, _ := repl.Get("http.reverse_proxy.upstream.hostport")
	s, _ := v.(string)
	return s
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
