package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// vitalsMetric holds fixed histogram upper bounds plus the public good and poor limits.
type vitalsMetric struct {
	bounds     []float64
	good, poor float64
}

// Bounds include each rating limit so a bucket never straddles a rating.
var vitalsMetrics = map[string]vitalsMetric{
	"LCP":  {bounds: []float64{500, 1000, 1500, 2000, 2500, 3000, 4000, 5000, 7500, 10000}, good: 2500, poor: 4000},
	"INP":  {bounds: []float64{50, 100, 150, 200, 300, 400, 500, 750, 1000, 2000}, good: 200, poor: 500},
	"CLS":  {bounds: []float64{0.01, 0.025, 0.05, 0.075, 0.1, 0.15, 0.2, 0.25, 0.5, 1}, good: 0.1, poor: 0.25},
	"TTFB": {bounds: []float64{100, 200, 400, 600, 800, 1200, 1800, 3000, 5000}, good: 800, poor: 1800},
	"FCP":  {bounds: []float64{500, 1000, 1500, 1800, 2200, 3000, 4000, 5000}, good: 1800, poor: 3000},
}

var vitalsMetricOrder = []string{"LCP", "INP", "CLS", "TTFB", "FCP"}

const (
	vitalsReportDays     = 7
	vitalsMaxRouteLen    = 120
	vitalsMaxMetricsSent = 10
	vitalsPruneEvery     = 10 * time.Minute
)

// vitalsStore is the optional store surface; the production *store.DB satisfies it.
type vitalsStore interface {
	GetVitalsConfig(ctx context.Context, app string) (store.VitalsConfig, bool, error)
	SetVitalsEnabled(ctx context.Context, app string, enabled bool) (store.VitalsConfig, error)
	RotateVitalsKey(ctx context.Context, app string, enabledIfNew bool) (store.VitalsConfig, error)
	AddVitalsHits(ctx context.Context, app, route, device string, at time.Time, hits []store.VitalsHit) error
	QueryVitals(ctx context.Context, app string, since time.Time) ([]store.VitalsBucketCount, error)
	PruneVitals(ctx context.Context, before time.Time) (int64, error)
}

func (rt *Router) vitalsDB() (vitalsStore, bool) {
	v, ok := rt.apps.(vitalsStore)
	return v, ok
}

// vitalsLimiter is a per-app fixed window kept in memory; nothing about callers is stored.
var vitalsLimiter = struct {
	sync.Mutex
	m map[string]*vitalsWindow
}{m: map[string]*vitalsWindow{}}

type vitalsWindow struct {
	start time.Time
	n     int
}

func vitalsAllow(app string, limit int, now time.Time) bool {
	vitalsLimiter.Lock()
	defer vitalsLimiter.Unlock()
	w := vitalsLimiter.m[app]
	if w == nil || now.Sub(w.start) >= time.Minute {
		if len(vitalsLimiter.m) > 10000 {
			clear(vitalsLimiter.m)
		}
		w = &vitalsWindow{start: now}
		vitalsLimiter.m[app] = w
	}
	w.n++
	return w.n <= limit
}

var vitalsLastPrune atomic.Int64

type vitalsBeacon struct {
	Key  string `json:"key"`
	Path string `json:"path"`
	M    []struct {
		N string  `json:"n"`
		V float64 `json:"v"`
	} `json:"m"`
}

func setVitalsCORS(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	h.Set("Access-Control-Max-Age", "600")
}

func (rt *Router) handleVitalsPreflight(w http.ResponseWriter, _ *http.Request) {
	setVitalsCORS(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleVitalsBeacon is the public collector. Unknown app, disabled app and wrong key all answer the same 404.
func (rt *Router) handleVitalsBeacon(w http.ResponseWriter, r *http.Request) {
	setVitalsCORS(w)
	db, ok := rt.vitalsDB()
	app := r.PathValue("app")
	if !ok || rt.runtimeSettings == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ctx := r.Context()
	if !vitalsAllow(app, rt.runtimeSettings.Int(ctx, KeyVitalsRatePerMin), time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many beacons")
		return
	}
	var b vitalsBeacon
	r.Body = http.MaxBytesReader(w, r.Body, int64(rt.runtimeSettings.Int(ctx, KeyVitalsMaxBody)))
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "beacon too large")
			return
		}
		writeError(w, http.StatusBadRequest, "malformed beacon")
		return
	}
	cfg, found, err := db.GetVitalsConfig(ctx, app)
	if err != nil {
		rt.internalError(w, "api: vitals beacon: load config", err)
		return
	}
	if !found || !cfg.Enabled || subtle.ConstantTimeCompare([]byte(cfg.PublicKey), []byte(b.Key)) != 1 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !rt.vitalsOriginAllowed(ctx, app, origin) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	hits := vitalsHits(b)
	if len(hits) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no valid metrics")
		return
	}
	now := time.Now()
	if err := db.AddVitalsHits(ctx, app, normalizeVitalsRoute(b.Path), vitalsDevice(r.UserAgent()), now, hits); err != nil {
		rt.internalError(w, "api: vitals beacon: store", err)
		return
	}
	rt.pruneVitals(ctx, db, now)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) pruneVitals(ctx context.Context, db vitalsStore, now time.Time) {
	last := vitalsLastPrune.Load()
	if now.UnixNano()-last < int64(vitalsPruneEvery) || !vitalsLastPrune.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	days := rt.runtimeSettings.Int(ctx, KeyVitalsRetentionDay)
	if _, err := db.PruneVitals(ctx, now.Add(-time.Duration(days)*24*time.Hour)); err != nil {
		rt.logger.Warn("api: prune vitals failed", "error", err.Error())
	}
}

// vitalsOriginAllowed accepts the app's own domains and localhost; an app with no domains accepts any origin.
func (rt *Router) vitalsOriginAllowed(ctx context.Context, app, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || rt.domains == nil {
		return true
	}
	doms, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return true
	}
	own := false
	for _, d := range doms {
		if d.ServiceName != app {
			continue
		}
		own = true
		if strings.EqualFold(d.Domain, host) {
			return true
		}
	}
	return !own
}

func vitalsHits(b vitalsBeacon) []store.VitalsHit {
	seen := map[string]bool{}
	var hits []store.VitalsHit
	for i, m := range b.M {
		spec, ok := vitalsMetrics[m.N]
		if i >= vitalsMaxMetricsSent || !ok || seen[m.N] || math.IsNaN(m.V) || math.IsInf(m.V, 0) || m.V < 0 {
			continue
		}
		seen[m.N] = true
		hits = append(hits, store.VitalsHit{Metric: m.N, Bucket: sort.SearchFloat64s(spec.bounds, m.V)})
	}
	return hits
}

// normalizeVitalsRoute drops query and fragment and folds id-like segments so routes stay low cardinality.
func normalizeVitalsRoute(p string) string {
	if u, err := url.Parse(p); err == nil {
		p = u.Path
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if vitalsIDSegment(s) {
			segs[i] = ":id"
		}
	}
	p = strings.Join(segs, "/")
	if len(p) > vitalsMaxRouteLen {
		p = p[:vitalsMaxRouteLen]
	}
	return p
}

func vitalsIDSegment(s string) bool {
	if s == "" {
		return false
	}
	digits, hexish := true, true
	for _, c := range s {
		if c < '0' || c > '9' {
			digits = false
		}
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') && c != '-' {
			hexish = false
		}
	}
	return digits || (hexish && len(s) >= 16)
}

// vitalsDevice reduces the user agent to a coarse class; the string itself is never kept.
func vitalsDevice(ua string) string {
	l := strings.ToLower(ua)
	if strings.Contains(l, "mobi") || strings.Contains(l, "android") || strings.Contains(l, "iphone") {
		return "mobile"
	}
	return "desktop"
}

type vitalsStat struct {
	Count  int64   `json:"count"`
	P50    float64 `json:"p50"`
	P75    float64 `json:"p75"`
	P95    float64 `json:"p95"`
	Rating string  `json:"rating"`
}

type vitalsRouteRow struct {
	Route   string                `json:"route"`
	Metrics map[string]vitalsStat `json:"metrics"`
}

type vitalsResponse struct {
	Enabled   bool                  `json:"enabled"`
	PublicKey string                `json:"public_key"`
	Days      int                   `json:"days"`
	Overall   map[string]vitalsStat `json:"overall"`
	Routes    []vitalsRouteRow      `json:"routes"`
}

func vitalsPercentile(metric string, buckets map[int]int64, total int64, p float64) float64 {
	spec := vitalsMetrics[metric]
	idx := make([]int, 0, len(buckets))
	for b := range buckets {
		idx = append(idx, b)
	}
	sort.Ints(idx)
	need := int64(math.Ceil(p * float64(total)))
	var cum int64
	for _, b := range idx {
		cum += buckets[b]
		if cum >= need {
			return spec.bounds[min(b, len(spec.bounds)-1)]
		}
	}
	return spec.bounds[len(spec.bounds)-1]
}

func vitalsStatOf(metric string, buckets map[int]int64) vitalsStat {
	var total int64
	for _, c := range buckets {
		total += c
	}
	st := vitalsStat{Count: total}
	if total == 0 {
		return st
	}
	st.P50 = vitalsPercentile(metric, buckets, total, 0.5)
	st.P75 = vitalsPercentile(metric, buckets, total, 0.75)
	st.P95 = vitalsPercentile(metric, buckets, total, 0.95)
	spec := vitalsMetrics[metric]
	switch {
	case st.P75 <= spec.good:
		st.Rating = "good"
	case st.P75 <= spec.poor:
		st.Rating = "needs-improvement"
	default:
		st.Rating = "poor"
	}
	return st
}

func buildVitalsReport(rows []store.VitalsBucketCount) (map[string]vitalsStat, []vitalsRouteRow) {
	byRoute := map[string]map[string]map[int]int64{}
	all := map[string]map[int]int64{}
	for _, c := range rows {
		if _, ok := vitalsMetrics[c.Metric]; !ok {
			continue
		}
		if byRoute[c.Route] == nil {
			byRoute[c.Route] = map[string]map[int]int64{}
		}
		for _, m := range []map[string]map[int]int64{byRoute[c.Route], all} {
			if m[c.Metric] == nil {
				m[c.Metric] = map[int]int64{}
			}
			m[c.Metric][c.Bucket] += c.Count
		}
	}
	overall := map[string]vitalsStat{}
	for m, b := range all {
		overall[m] = vitalsStatOf(m, b)
	}
	routes := make([]vitalsRouteRow, 0, len(byRoute))
	for route, ms := range byRoute {
		row := vitalsRouteRow{Route: route, Metrics: map[string]vitalsStat{}}
		for m, b := range ms {
			row.Metrics[m] = vitalsStatOf(m, b)
		}
		routes = append(routes, row)
	}
	// Busiest routes first, by LCP sample count with page views as the tiebreak.
	sort.Slice(routes, func(i, j int) bool {
		ci, cj := routes[i].Metrics["LCP"].Count, routes[j].Metrics["LCP"].Count
		if ci != cj {
			return ci > cj
		}
		return routes[i].Route < routes[j].Route
	})
	return overall, routes
}

func (rt *Router) vitalsApp(w http.ResponseWriter, r *http.Request) (vitalsStore, string, bool) {
	db, ok := rt.vitalsDB()
	if !ok {
		writeError(w, http.StatusNotImplemented, "web vitals is not available")
		return nil, "", false
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, "", false
	} else if err != nil {
		rt.internalError(w, "api: vitals: load app", err)
		return nil, "", false
	}
	return db, name, true
}

func (rt *Router) writeVitals(w http.ResponseWriter, r *http.Request, db vitalsStore, name string) {
	ctx := r.Context()
	cfg, found, err := db.GetVitalsConfig(ctx, name)
	if err != nil {
		rt.internalError(w, "api: vitals: load config", err)
		return
	}
	resp := vitalsResponse{Enabled: rt.runtimeSettings.Bool(ctx, KeyVitalsEnabled), Days: vitalsReportDays}
	if found {
		resp.Enabled, resp.PublicKey = cfg.Enabled, cfg.PublicKey
	}
	rows, err := db.QueryVitals(ctx, name, time.Now().Add(-vitalsReportDays*24*time.Hour))
	if err != nil {
		rt.internalError(w, "api: vitals: query", err)
		return
	}
	resp.Overall, resp.Routes = buildVitalsReport(rows)
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) handleGetVitals(w http.ResponseWriter, r *http.Request) {
	if db, name, ok := rt.vitalsApp(w, r); ok {
		rt.writeVitals(w, r, db, name)
	}
}

func (rt *Router) handleSetVitals(w http.ResponseWriter, r *http.Request) {
	db, name, ok := rt.vitalsApp(w, r)
	if !ok {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body")
		return
	}
	if _, err := db.SetVitalsEnabled(r.Context(), name, req.Enabled); err != nil {
		rt.internalError(w, "api: vitals: set enabled", err)
		return
	}
	rt.writeVitals(w, r, db, name)
}

func (rt *Router) handleRotateVitalsKey(w http.ResponseWriter, r *http.Request) {
	db, name, ok := rt.vitalsApp(w, r)
	if !ok {
		return
	}
	if _, err := db.RotateVitalsKey(r.Context(), name, rt.runtimeSettings.Bool(r.Context(), KeyVitalsEnabled)); err != nil {
		rt.internalError(w, "api: vitals: rotate key", err)
		return
	}
	rt.writeVitals(w, r, db, name)
}
