package telemetry

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Breakdown env vars.
const (
	// EnvRequestRoutes set to "off" disables per-route request counting.
	EnvRequestRoutes = "APP_REQUEST_ROUTES"
	// EnvRequestRouteMax caps distinct routes per host per sampling tick.
	EnvRequestRouteMax = "APP_REQUEST_ROUTE_MAX"
	// EnvRequestRouteDepth caps path segments kept in a route template.
	EnvRequestRouteDepth = "APP_REQUEST_ROUTE_DEPTH"
)

const (
	defaultRouteMax   = 50
	defaultRouteDepth = 3
	// OverflowRoute collects traffic beyond the per-tick route cap.
	OverflowRoute = "other"
	// maxStatusKeys bounds distinct exact status codes per tick.
	maxStatusKeys = 64

	breakdownKindRoute  = "route"
	breakdownKindStatus = "status"
	breakdownBucket     = time.Minute
)

// RouteStat is one route's counters over a sampling tick.
type RouteStat struct {
	Requests     uint64
	Errors4xx    uint64
	Errors5xx    uint64
	LatencyMSSum float64
}

// RoutesEnabled reports whether per-route counting is on.
func RoutesEnabled() bool {
	return !strings.EqualFold(os.Getenv(EnvRequestRoutes), "off")
}

// RouteMaxFromEnv returns the per-tick route cap.
func RouteMaxFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv(EnvRequestRouteMax)); err == nil && n > 0 {
		return n
	}
	return defaultRouteMax
}

// RouteDepthFromEnv returns how many path segments a route template keeps.
func RouteDepthFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv(EnvRequestRouteDepth)); err == nil && n > 0 {
		return n
	}
	return defaultRouteDepth
}

// NormalizeRoute reduces a URL path to a bounded template: query strings are
// never passed in, at most depth segments are kept, and any segment that looks
// like an identifier (digits, hex, UUID, long or mixed alphanumerics) becomes
// ":id" so tokens and user ids never reach storage.
func NormalizeRoute(path string, depth int) string {
	if depth <= 0 {
		depth = defaultRouteDepth
	}
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/"
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, depth+1)
	for i, p := range parts {
		if i >= depth {
			out = append(out, "*")
			break
		}
		out = append(out, normalizeSegment(p))
	}
	return "/" + strings.Join(out, "/")
}

func normalizeSegment(s string) string {
	if s == "" {
		return s
	}
	if len(s) > 24 {
		return ":id"
	}
	digits := 0
	allHex := true
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r >= 'a' && r <= 'f', r >= 'A' && r <= 'F', r == '-', r == '_', r == '.':
		default:
			allHex = false
		}
	}
	switch {
	case digits == len(s):
		return ":id"
	case allHex && len(s) >= 8 && digits > 0:
		return ":id"
	case digits >= 3 && digits*3 >= len(s):
		return ":id"
	}
	return s
}

// ObserveRoute records one request into the window's route and status maps.
func (w *RequestWindow) ObserveRoute(route string, status int, latencyMS float64, routeMax int) {
	if w.Statuses == nil {
		w.Statuses = make(map[int]uint64)
	}
	if _, ok := w.Statuses[status]; ok || len(w.Statuses) < maxStatusKeys {
		w.Statuses[status]++
	}
	if route == "" {
		return
	}
	if w.Routes == nil {
		w.Routes = make(map[string]RouteStat)
	}
	rs, ok := w.Routes[route]
	if !ok && routeMax > 0 && len(w.Routes) >= routeMax {
		route = OverflowRoute
		rs = w.Routes[route]
	}
	rs.Requests++
	switch {
	case status >= 500:
		rs.Errors5xx++
	case status >= 400:
		rs.Errors4xx++
	}
	rs.LatencyMSSum += latencyMS
	w.Routes[route] = rs
}

func (w *RequestWindow) mergeBreakdown(o RequestWindow) {
	for code, n := range o.Statuses {
		if w.Statuses == nil {
			w.Statuses = make(map[int]uint64)
		}
		w.Statuses[code] += n
	}
	for r, s := range o.Routes {
		if w.Routes == nil {
			w.Routes = make(map[string]RouteStat)
		}
		cur := w.Routes[r]
		cur.Requests += s.Requests
		cur.Errors4xx += s.Errors4xx
		cur.Errors5xx += s.Errors5xx
		cur.LatencyMSSum += s.LatencyMSSum
		w.Routes[r] = cur
	}
}

// RouteRow is one route's totals over a query window.
type RouteRow struct {
	Route        string
	Requests     float64
	Errors4xx    float64
	Errors5xx    float64
	LatencyMSSum float64
}

// StatusRow is one exact status code's total over a query window.
type StatusRow struct {
	Status int
	Count  float64
}

// Breakdown is the route and status totals for one app and window.
type Breakdown struct {
	Routes   []RouteRow
	Statuses []StatusRow
}

// BreakdownSource reads route and status totals; *DB satisfies it.
type BreakdownSource interface {
	QueryBreakdown(ctx context.Context, resourceID string, from, to time.Time) (Breakdown, error)
}

// sat converts a counter to int64, saturating instead of wrapping.
func sat(n uint64) int64 {
	if n > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(n)
}

func (db *DB) recordBreakdown(ctx context.Context, byApp map[string]RequestWindow, at time.Time) error {
	bucket := at.Truncate(breakdownBucket).Unix()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin breakdown write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	const upsert = `
		INSERT INTO request_breakdown (resource_id, kind, key, ts, requests, errors_4xx, errors_5xx, latency_ms_sum)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (resource_id, kind, key, ts) DO UPDATE SET
			requests = requests + excluded.requests,
			errors_4xx = errors_4xx + excluded.errors_4xx,
			errors_5xx = errors_5xx + excluded.errors_5xx,
			latency_ms_sum = latency_ms_sum + excluded.latency_ms_sum`
	wrote := false
	for app, w := range byApp {
		rid := "service:" + app
		for route, s := range w.Routes {
			if _, err := tx.ExecContext(ctx, upsert, rid, breakdownKindRoute, route, bucket,
				sat(s.Requests), sat(s.Errors4xx), sat(s.Errors5xx), s.LatencyMSSum); err != nil {
				return fmt.Errorf("telemetry: write route breakdown: %w", err)
			}
			wrote = true
		}
		for code, n := range w.Statuses {
			if _, err := tx.ExecContext(ctx, upsert, rid, breakdownKindStatus, strconv.Itoa(code), bucket,
				sat(n), 0, 0, 0.0); err != nil {
				return fmt.Errorf("telemetry: write status breakdown: %w", err)
			}
			wrote = true
		}
	}
	if !wrote {
		return nil
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("telemetry: commit breakdown: %w", err)
	}
	return nil
}

// QueryBreakdown sums route and status counters for resourceID in [from, to].
func (db *DB) QueryBreakdown(ctx context.Context, resourceID string, from, to time.Time) (Breakdown, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT kind, key, SUM(requests), SUM(errors_4xx), SUM(errors_5xx), SUM(latency_ms_sum)
		FROM request_breakdown
		WHERE resource_id = ? AND ts BETWEEN ? AND ?
		GROUP BY kind, key`,
		resourceID, from.Truncate(breakdownBucket).Unix(), to.Unix())
	if err != nil {
		return Breakdown{}, fmt.Errorf("telemetry: query breakdown for %s: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()
	var out Breakdown
	for rows.Next() {
		var kind, key string
		var req, e4, e5 int64
		var lat float64
		if err := rows.Scan(&kind, &key, &req, &e4, &e5, &lat); err != nil {
			return Breakdown{}, fmt.Errorf("telemetry: scan breakdown: %w", err)
		}
		switch kind {
		case breakdownKindRoute:
			out.Routes = append(out.Routes, RouteRow{Route: key, Requests: float64(req), Errors4xx: float64(e4), Errors5xx: float64(e5), LatencyMSSum: lat})
		case breakdownKindStatus:
			if code, err := strconv.Atoi(key); err == nil {
				out.Statuses = append(out.Statuses, StatusRow{Status: code, Count: float64(req)})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Breakdown{}, fmt.Errorf("telemetry: iterate breakdown: %w", err)
	}
	return out, nil
}

// QueryBreakdown merges route and status totals from every source that
// implements BreakdownSource, tolerating partial failure like QueryMetrics.
func (f *Federator) QueryBreakdown(ctx context.Context, resourceID string, from, to time.Time) (Breakdown, error) {
	routes := map[string]*RouteRow{}
	statuses := map[int]float64{}
	var firstErr error
	for _, src := range f.metrics {
		bs, ok := src.(BreakdownSource)
		if !ok {
			continue
		}
		b, err := bs.QueryBreakdown(ctx, resourceID, from, to)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, r := range b.Routes {
			cur := routes[r.Route]
			if cur == nil {
				cur = &RouteRow{Route: r.Route}
				routes[r.Route] = cur
			}
			cur.Requests += r.Requests
			cur.Errors4xx += r.Errors4xx
			cur.Errors5xx += r.Errors5xx
			cur.LatencyMSSum += r.LatencyMSSum
		}
		for _, s := range b.Statuses {
			statuses[s.Status] += s.Count
		}
	}
	var merged Breakdown
	for _, r := range routes {
		merged.Routes = append(merged.Routes, *r)
	}
	for code, n := range statuses {
		merged.Statuses = append(merged.Statuses, StatusRow{Status: code, Count: n})
	}
	return merged, firstErr
}

// TopRoutes returns the n busiest routes, requests descending, ties by name.
func (b Breakdown) TopRoutes(n int) []RouteRow {
	rows := append([]RouteRow(nil), b.Routes...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Requests != rows[j].Requests {
			return rows[i].Requests > rows[j].Requests
		}
		return rows[i].Route < rows[j].Route
	})
	if n > 0 && len(rows) > n {
		rows = rows[:n]
	}
	return rows
}

// SortedStatuses returns status totals by count descending, ties by code.
func (b Breakdown) SortedStatuses() []StatusRow {
	rows := append([]StatusRow(nil), b.Statuses...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Status < rows[j].Status
	})
	return rows
}

// Total returns the request count, preferring exact status totals.
func (b Breakdown) Total() float64 {
	var t float64
	for _, s := range b.Statuses {
		t += s.Count
	}
	if t > 0 {
		return t
	}
	for _, r := range b.Routes {
		t += r.Requests
	}
	return t
}
