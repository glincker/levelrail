package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func init() {
	tenantRoutes["GET /api/v1/sleep/overview"] = true
}

const (
	sleepSourceMaxLen  = 128
	sleepHistoryLimit  = 100
	sleepTopSaversKept = 5
	bytesPerGiB        = float64(1 << 30)
)

// sleepEventStore is the history surface of the store; *store.DB satisfies it.
type sleepEventStore interface {
	appsleep.EventStore
	SetSleepEventLatency(ctx context.Context, id int64, latencyMs int64) error
	ListSleepEvents(ctx context.Context, name string, since time.Time, limit int) ([]store.SleepEvent, error)
	GetSleepStats(ctx context.Context, name string, now time.Time) (store.SleepStats, error)
}

func (rt *Router) sleepEvents() sleepEventStore {
	s, _ := rt.appSleep.(sleepEventStore)
	return s
}

func (rt *Router) recordSleepEvent(ctx context.Context, e store.SleepEvent) int64 {
	s := rt.sleepEvents()
	if s == nil {
		return 0
	}
	return appsleep.Record(ctx, s, rt.logger, e)
}

// sleepRequestSource labels who triggered a wake without keeping a client
// address: a probe or bot guess from the path and user agent, else the host asked for.
func sleepRequestSource(host, path, userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case containsAny(ua, "healthcheck", "health-check", "kube-probe", "uptime", "pingdom", "probe", "monitor"),
		containsAny(path, "/health", "/healthz", "/ping", "/readyz", "/livez", "/status"):
		return appsleep.SourceHealthProbe
	case containsAny(ua, "bot", "crawler", "spider", "slurp"):
		return appsleep.SourceBot
	}
	if len(host) > sleepSourceMaxLen {
		host = host[:sleepSourceMaxLen]
	}
	return host
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// wakeRequestEvent builds the woke row for a request that hit a sleeping app.
func wakeRequestEvent(r *http.Request, name string, slept, now time.Time) store.SleepEvent {
	e := appsleep.WokeEvent(name, store.SleepReasonRequest, "system", slept, now)
	if u, err := url.ParseRequestURI(r.Header.Get(ingress.WakeURIHeader)); err == nil {
		e.TriggerPath = u.Path
	}
	e.TriggerSource = sleepRequestSource(r.Host, e.TriggerPath, r.UserAgent())
	return e
}

// finishWake stamps the wake's latency once the app is healthy, or records a
// wake_failed row when it never became ready within the hold timeout.
func (rt *Router) finishWake(ctx context.Context, name string, id int64, start time.Time, hold time.Duration, ready bool, reason string) {
	s := rt.sleepEvents()
	if s == nil || id == 0 {
		return
	}
	if ready {
		if err := s.SetSleepEventLatency(ctx, id, time.Since(start).Milliseconds()); err != nil {
			rt.logger.Warn("api: record wake latency failed", slog.String("app", name), slog.String("error", err.Error()))
		}
		return
	}
	rt.recordSleepEvent(ctx, store.SleepEvent{ServiceName: name, Kind: store.SleepEventWakeFailed, At: time.Now(), Reason: reason, Detail: "app was not healthy within " + hold.String(), Actor: "system"})
}

// watchWake measures a wake's latency in the background for callers that
// answer before the app is ready.
func (rt *Router) watchWake(name string, id int64, start time.Time, reason string) {
	if id == 0 {
		return
	}
	cfg := appsleep.ConfigFromEnv(nil)
	hold := cfg.HoldTimeout(rt.wakeTiming(context.Background(), name).ReadyP95Ms)
	go func() {
		ctx := context.Background()
		rt.finishWake(ctx, name, id, start, hold, rt.holdUntilReady(ctx, name, start, hold, cfg.WakeProbeInterval), reason)
	}()
}

type sleepEventResource struct {
	ID              int64     `json:"id"`
	Kind            string    `json:"kind"`
	At              time.Time `json:"at"`
	Reason          string    `json:"reason,omitempty"`
	IdleMinutes     int       `json:"idle_minutes,omitempty"`
	TriggerPath     string    `json:"trigger_path,omitempty"`
	TriggerSource   string    `json:"trigger_source,omitempty"`
	Detail          string    `json:"detail,omitempty"`
	DurationSeconds *int64    `json:"duration_seconds,omitempty"`
	WakeLatencyMs   *int64    `json:"wake_latency_ms,omitempty"`
	Actor           string    `json:"actor"`
}

type sleepStatsResource struct {
	CurrentlyAsleepSince *time.Time `json:"currently_asleep_since,omitempty"`
	AsleepSeconds7d      int64      `json:"asleep_seconds_7d"`
	AsleepSeconds30d     int64      `json:"asleep_seconds_30d"`
	WakeCount7d          int        `json:"wake_count_7d"`
	AvgWakeLatencyMs     *int64     `json:"avg_wake_latency_ms,omitempty"`
	P95WakeLatencyMs     *int64     `json:"p95_wake_latency_ms,omitempty"`
	LastSleptAt          *time.Time `json:"last_slept_at,omitempty"`
	LastWokeAt           *time.Time `json:"last_woke_at,omitempty"`
	LastWakeReason       string     `json:"last_wake_reason,omitempty"`
	// MemoryGiBHoursSaved30d is the memory limit times asleep hours; absent without a limit.
	MemoryGiBHoursSaved30d *float64 `json:"memory_gib_hours_saved_30d,omitempty"`
}

type sleepHistoryResource struct {
	Enabled  bool                 `json:"enabled"`
	Sleeping bool                 `json:"sleeping"`
	Events   []sleepEventResource `json:"events"`
	Stats    sleepStatsResource   `json:"stats"`
	// WakeTiming covers the app's most recent timed wakes, not the requested window.
	WakeTiming wakeTimingResource `json:"wake_timing"`
	IdleLimits sleepIdleLimits    `json:"idle_limits"`
}

func toSleepStatsResource(st store.SleepStats, memLimit int64) sleepStatsResource {
	out := sleepStatsResource{
		CurrentlyAsleepSince: st.CurrentlyAsleepSince, AsleepSeconds7d: st.AsleepSeconds7d, AsleepSeconds30d: st.AsleepSeconds30d,
		WakeCount7d: st.WakeCount7d, AvgWakeLatencyMs: st.AvgWakeLatencyMs, P95WakeLatencyMs: st.P95WakeLatencyMs,
		LastSleptAt: st.LastSleptAt, LastWokeAt: st.LastWokeAt, LastWakeReason: st.LastWakeReason,
	}
	if memLimit > 0 {
		v := float64(memLimit) / bytesPerGiB * float64(st.AsleepSeconds30d) / 3600
		out.MemoryGiBHoursSaved30d = &v
	}
	return out
}

func toSleepEventResource(e store.SleepEvent) sleepEventResource {
	return sleepEventResource{ID: e.ID, Kind: e.Kind, At: e.At.UTC(), Reason: e.Reason, IdleMinutes: e.IdleMinutes, TriggerPath: e.TriggerPath,
		TriggerSource: e.TriggerSource, Detail: e.Detail, DurationSeconds: e.DurationSeconds, WakeLatencyMs: e.WakeLatencyMs, Actor: e.Actor}
}

// parseSleepSince reads "7d", "36h" or an RFC3339 time; empty means no bound.
func parseSleepSince(raw string, now time.Time) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, true
	}
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return time.Time{}, false
		}
		return now.Add(-time.Duration(n) * 24 * time.Hour), true
	}
	if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
		return now.Add(-d), true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// handleAppSleepHistory handles GET /api/v1/apps/{name}/sleep/history.
func (rt *Router) handleAppSleepHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s := rt.sleepEvents()
	if s == nil {
		writeError(w, http.StatusNotImplemented, "sleep history is not available")
		return
	}
	now := time.Now()
	since, ok := parseSleepSince(r.URL.Query().Get("since"), now)
	if !ok {
		writeError(w, http.StatusBadRequest, "since must be a duration like 7d or 36h, or an RFC3339 time")
		return
	}
	limit := sleepHistoryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive number")
			return
		}
		limit = n
	}
	events, err := s.ListSleepEvents(r.Context(), name, since, limit)
	if err != nil {
		rt.internalError(w, "api: sleep history: list", err, slog.String("name", name))
		return
	}
	stats, err := s.GetSleepStats(r.Context(), name, now)
	if err != nil {
		rt.internalError(w, "api: sleep history: stats", err, slog.String("name", name))
		return
	}
	setting, err := rt.appSleep.GetAppSleep(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: sleep history: setting", err, slog.String("name", name))
		return
	}
	var memLimit int64
	if svc, err := rt.apps.GetDesiredService(r.Context(), name); err == nil && svc.Resources != nil {
		memLimit = svc.Resources.MemoryBytes
	}
	out := sleepHistoryResource{Enabled: setting.IdleMinutes > 0, Sleeping: setting.Sleeping, Events: make([]sleepEventResource, 0, len(events)), Stats: toSleepStatsResource(stats, memLimit), WakeTiming: toWakeTimingResource(rt.wakeTiming(r.Context(), name)), IdleLimits: currentIdleLimits()}
	for _, e := range events {
		out.Events = append(out.Events, toSleepEventResource(e))
	}
	writeJSON(w, http.StatusOK, out)
}

type sleepOverviewApp struct {
	Name             string     `json:"name"`
	Since            *time.Time `json:"since,omitempty"`
	IdleMinutes      int        `json:"idle_minutes"`
	MemoryLimitBytes int64      `json:"memory_limit_bytes"`
}

type sleepSaver struct {
	Name            string   `json:"name"`
	AsleepSeconds7d int64    `json:"asleep_seconds_7d"`
	WakeCount7d     int      `json:"wake_count_7d"`
	GiBHoursSaved7d *float64 `json:"memory_gib_hours_saved_7d,omitempty"`
}

type sleepOverviewResource struct {
	Sleeping  []sleepOverviewApp `json:"sleeping"`
	TopSavers []sleepSaver       `json:"top_savers"`
}

// handleSleepOverview handles GET /api/v1/sleep/overview: apps asleep now
// with since, and the apps that spent the most of the last 7 days asleep.
func (rt *Router) handleSleepOverview(w http.ResponseWriter, r *http.Request) {
	s := rt.sleepEvents()
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: sleep overview: resolve caller", err)
		return
	}
	rows, err := rt.appSleep.ListAppSleep(r.Context())
	if err != nil {
		rt.internalError(w, "api: sleep overview: list", err)
		return
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: sleep overview: list apps", err)
		return
	}
	limits := make(map[string]int64, len(svcs))
	for _, sv := range svcs {
		if sv.Resources != nil {
			limits[sv.Name] = sv.Resources.MemoryBytes
		}
	}
	out := sleepOverviewResource{Sleeping: []sleepOverviewApp{}, TopSavers: []sleepSaver{}}
	now := time.Now()
	for _, a := range rows {
		if !canRead(a.ServiceName) {
			continue
		}
		if a.Sleeping {
			since := a.Since
			out.Sleeping = append(out.Sleeping, sleepOverviewApp{Name: a.ServiceName, Since: &since, IdleMinutes: a.IdleMinutes, MemoryLimitBytes: limits[a.ServiceName]})
		}
		if s == nil {
			continue
		}
		st, err := s.GetSleepStats(r.Context(), a.ServiceName, now)
		if err != nil {
			rt.logger.Warn("api: sleep overview: stats failed", slog.String("app", a.ServiceName), slog.String("error", err.Error()))
			continue
		}
		if st.AsleepSeconds7d == 0 {
			continue
		}
		saver := sleepSaver{Name: a.ServiceName, AsleepSeconds7d: st.AsleepSeconds7d, WakeCount7d: st.WakeCount7d}
		if lim := limits[a.ServiceName]; lim > 0 {
			v := float64(lim) / bytesPerGiB * float64(st.AsleepSeconds7d) / 3600
			saver.GiBHoursSaved7d = &v
		}
		out.TopSavers = append(out.TopSavers, saver)
	}
	sort.Slice(out.Sleeping, func(i, j int) bool { return out.Sleeping[i].Since.Before(*out.Sleeping[j].Since) })
	sort.Slice(out.TopSavers, func(i, j int) bool {
		if out.TopSavers[i].AsleepSeconds7d != out.TopSavers[j].AsleepSeconds7d {
			return out.TopSavers[i].AsleepSeconds7d > out.TopSavers[j].AsleepSeconds7d
		}
		return out.TopSavers[i].Name < out.TopSavers[j].Name
	})
	if len(out.TopSavers) > sleepTopSaversKept {
		out.TopSavers = out.TopSavers[:sleepTopSaversKept]
	}
	writeJSON(w, http.StatusOK, out)
}
