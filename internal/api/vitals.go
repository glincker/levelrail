package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
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
	vitalsMaxMetricsSent = 10
)

// vitalsStore is the optional store surface; the production *store.DB satisfies it.
type vitalsStore interface {
	GetVitalsConfig(ctx context.Context, app string) (store.VitalsConfig, bool, error)
	SetVitalsEnabled(ctx context.Context, app string, enabled bool) (store.VitalsConfig, error)
	RotateVitalsKey(ctx context.Context, app string, enabledIfNew bool) (store.VitalsConfig, error)
	AddVitalsCounts(ctx context.Context, rows []store.VitalsCount) error
	VitalsRoutesSince(ctx context.Context, app string, since time.Time, limit int) ([]string, error)
	QueryVitals(ctx context.Context, app string, since time.Time, topRoutes int) ([]store.VitalsBucketCount, error)
	PruneVitalsBatch(ctx context.Context, before time.Time, limit int) (int64, error)
}

func (rt *Router) vitalsDB() (vitalsStore, bool) {
	v, ok := rt.apps.(vitalsStore)
	return v, ok
}

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
	v := rt.vitalsRuntime()
	if ok, _ := v.perIP.allow(clientIP(r)); !ok {
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
	if _, err := rt.apps.GetDesiredService(ctx, app); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: vitals beacon: load app", err)
		return
	}
	// Charged only after the key matched, so junk requests cannot burn an app's quota.
	if ok, _ := v.perApp.allow(app); !ok {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many beacons")
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
	route := v.gate.admit(ctx, db, app, normalizeVitalsRoute(b.Path), rt.runtimeSettings.Int(ctx, KeyVitalsMaxRoutes), now)
	device, hour := vitalsDevice(r.UserAgent()), now.UTC().Truncate(time.Hour)
	for _, h := range hits {
		v.buf.add(vitalsKey{app: app, route: route, metric: h.Metric, device: device, hour: hour, bucket: h.Bucket}, 1)
	}
	w.WriteHeader(http.StatusNoContent)
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

// vitalsDevice reduces the user agent to a coarse class; the string itself is never kept.
func vitalsDevice(ua string) string {
	l := strings.ToLower(ua)
	if strings.Contains(l, "mobi") || strings.Contains(l, "android") || strings.Contains(l, "iphone") {
		return "mobile"
	}
	return "desktop"
}

type vitalsStat struct {
	Count int64   `json:"count"`
	P50   float64 `json:"p50"`
	P75   float64 `json:"p75"`
	P95   float64 `json:"p95"`
	// Overflow means the percentile lies above the top histogram bound, so the value is a lower bound.
	Overflow    bool   `json:"overflow,omitempty"`
	P95Overflow bool   `json:"p95_overflow,omitempty"`
	Rating      string `json:"rating"`
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

func vitalsPercentile(metric string, buckets map[int]int64, total int64, p float64) (val float64, overflow bool) {
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
			return spec.bounds[min(b, len(spec.bounds)-1)], b >= len(spec.bounds)
		}
	}
	return spec.bounds[len(spec.bounds)-1], true
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
	st.P50, _ = vitalsPercentile(metric, buckets, total, 0.5)
	st.P75, st.Overflow = vitalsPercentile(metric, buckets, total, 0.75)
	st.P95, st.P95Overflow = vitalsPercentile(metric, buckets, total, 0.95)
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
	resp := vitalsResponse{Days: vitalsReportDays}
	if found {
		resp.Enabled, resp.PublicKey = cfg.Enabled, cfg.PublicKey
	}
	rows, err := db.QueryVitals(ctx, name, time.Now().Add(-vitalsReportDays*24*time.Hour), rt.runtimeSettings.Int(ctx, KeyVitalsTopRoutes))
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
