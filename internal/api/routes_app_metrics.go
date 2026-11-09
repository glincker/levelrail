package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/metrics"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appMetricsStore keeps scrape targets and scraped samples; *store.DB satisfies it.
type appMetricsStore interface {
	SetAppMetricsTarget(ctx context.Context, t store.AppMetricsTarget) error
	GetAppMetricsTarget(ctx context.Context, app string) (store.AppMetricsTarget, error)
	DeleteAppMetricsTarget(ctx context.Context, app string) error
	ListAppMetricsTargets(ctx context.Context) ([]store.AppMetricsTarget, error)
	RecordAppMetricsScrapeError(ctx context.Context, app, msg string) error
	InsertAppMetricSamples(ctx context.Context, app string, samples []store.AppMetricSample, maxRows int) error
	ListAppMetricNames(ctx context.Context, app string, since time.Time, limit int) ([]string, error)
	QueryAppMetricSamples(ctx context.Context, app, name string, from, to time.Time, limit int) ([]store.AppMetricSample, error)
	PruneAppMetricSamples(ctx context.Context, before time.Time) (int64, error)
}

func (rt *Router) appMetricsDB() appMetricsStore {
	s, _ := rt.apps.(appMetricsStore)
	return s
}

func (rt *Router) registerAppMetricsRoutes(mux *http.ServeMux) {
	// Scrape target of an app's own metrics endpoint: port and path on its container only, never a host.
	mux.HandleFunc("GET /api/v1/apps/{name}/scrape-target", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetScrapeTarget))
	mux.HandleFunc("PUT /api/v1/apps/{name}/scrape-target", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handlePutScrapeTarget))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/scrape-target", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteScrapeTarget))
	// Scraped series of one app: names, or one metric's points grouped by label set.
	mux.HandleFunc("GET /api/v1/apps/{name}/scraped-metrics", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleScrapedMetrics))
}

type scrapeTargetBody struct {
	Configured bool   `json:"configured"`
	Port       int    `json:"port"`
	Path       string `json:"path"`
	LastError  string `json:"last_error,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type scrapedPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type scrapedSeries struct {
	Labels string         `json:"labels"`
	Points []scrapedPoint `json:"points"`
}

type scrapedMetricsBody struct {
	Names  []string        `json:"names,omitempty"`
	Metric string          `json:"metric,omitempty"`
	Series []scrapedSeries `json:"series,omitempty"`
}

// appMetricsApp resolves the path app and the store, writing the error response itself.
func (rt *Router) appMetricsApp(w http.ResponseWriter, r *http.Request) (store.DesiredService, appMetricsStore, bool) {
	db := rt.appMetricsDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "app metrics are not available on this control plane")
		return store.DesiredService{}, nil, false
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return store.DesiredService{}, nil, false
	} else if err != nil {
		rt.internalError(w, "api: app metrics: load app failed", err)
		return store.DesiredService{}, nil, false
	}
	return *svc, db, true
}

func (rt *Router) handleGetScrapeTarget(w http.ResponseWriter, r *http.Request) {
	svc, db, ok := rt.appMetricsApp(w, r)
	if !ok {
		return
	}
	t, err := db.GetAppMetricsTarget(r.Context(), svc.Name)
	if errors.Is(err, store.ErrAppMetricsTargetNotFound) {
		writeJSON(w, http.StatusOK, scrapeTargetBody{})
		return
	} else if err != nil {
		rt.internalError(w, "api: get scrape target failed", err)
		return
	}
	writeJSON(w, http.StatusOK, scrapeTargetBody{Configured: true, Port: t.Port, Path: t.Path, LastError: t.LastError, UpdatedAt: t.UpdatedAt})
}

func (rt *Router) handlePutScrapeTarget(w http.ResponseWriter, r *http.Request) {
	svc, db, ok := rt.appMetricsApp(w, r)
	if !ok {
		return
	}
	var in struct {
		Port int    `json:"port"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := metrics.ValidatePort(in.Port); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := metrics.ValidatePath(in.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.SetAppMetricsTarget(r.Context(), store.AppMetricsTarget{App: svc.Name, Port: in.Port, Path: in.Path}); err != nil {
		rt.internalError(w, "api: set scrape target failed", err)
		return
	}
	writeJSON(w, http.StatusOK, scrapeTargetBody{Configured: true, Port: in.Port, Path: in.Path})
}

func (rt *Router) handleDeleteScrapeTarget(w http.ResponseWriter, r *http.Request) {
	svc, db, ok := rt.appMetricsApp(w, r)
	if !ok {
		return
	}
	if err := db.DeleteAppMetricsTarget(r.Context(), svc.Name); err != nil {
		rt.internalError(w, "api: delete scrape target failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleScrapedMetrics(w http.ResponseWriter, r *http.Request) {
	svc, db, ok := rt.appMetricsApp(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	limit := max(rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppMetricsQueryMaxPoints), 1)
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		since := time.Now().Add(-time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppMetricsRetentionHours)) * time.Hour)
		names, err := db.ListAppMetricNames(ctx, svc.Name, since, max(rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppMetricsMaxSeries), 1))
		if err != nil {
			rt.internalError(w, "api: list scraped metric names failed", err)
			return
		}
		writeJSON(w, http.StatusOK, scrapedMetricsBody{Names: names})
		return
	}
	if !metrics.ValidName(metric) {
		writeError(w, http.StatusBadRequest, "metric is not a valid metric name")
		return
	}
	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	samples, err := db.QueryAppMetricSamples(ctx, svc.Name, metric, from, to, limit)
	if err != nil {
		rt.internalError(w, "api: query scraped metric failed", err)
		return
	}
	writeJSON(w, http.StatusOK, scrapedMetricsBody{Metric: metric, Series: groupScraped(samples)})
}

func groupScraped(samples []store.AppMetricSample) []scrapedSeries {
	idx := map[string]int{}
	out := []scrapedSeries{}
	for _, s := range samples {
		i, ok := idx[s.Labels]
		if !ok {
			i = len(out)
			idx[s.Labels] = i
			out = append(out, scrapedSeries{Labels: s.Labels})
		}
		out[i].Points = append(out[i].Points, scrapedPoint{Timestamp: s.At, Value: s.Value})
	}
	return out
}

// RunAppMetricsScraper scrapes every configured app on the interval from runtime settings.
func (rt *Router) RunAppMetricsScraper(ctx context.Context) error {
	db := rt.appMetricsDB()
	if db == nil || rt.runtimeSettings == nil {
		return nil
	}
	for {
		wait := max(rt.runtimeSettings.Duration(ctx, runtimesettings.KeyAppMetricsScrapeInterval), 15*time.Second)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		rt.scrapeAppsOnce(ctx, db)
	}
}

func (rt *Router) appMetricsLimits(ctx context.Context) metrics.Limits {
	s := rt.runtimeSettings
	return metrics.Limits{
		Timeout:       s.Duration(ctx, runtimesettings.KeyAppMetricsScrapeTimeout),
		MaxBodyBytes:  int64(s.Int(ctx, runtimesettings.KeyAppMetricsMaxBodyBytes)),
		MaxSeries:     s.Int(ctx, runtimesettings.KeyAppMetricsMaxSeries),
		MaxLabels:     s.Int(ctx, runtimesettings.KeyAppMetricsMaxLabels),
		MaxLabelBytes: s.Int(ctx, runtimesettings.KeyAppMetricsMaxLabelBytes),
	}
}

func (rt *Router) scrapeAppsOnce(ctx context.Context, db appMetricsStore) {
	hours := rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppMetricsRetentionHours)
	if _, err := db.PruneAppMetricSamples(ctx, time.Now().Add(-time.Duration(hours)*time.Hour)); err != nil {
		rt.logger.Warn("api: prune app metrics failed", slog.String("error", err.Error()))
	}
	targets, err := db.ListAppMetricsTargets(ctx)
	if err != nil || len(targets) == 0 {
		return
	}
	lim := rt.appMetricsLimits(ctx)
	maxRows := rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppMetricsMaxStoredRows)
	workers := min(max(rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppMetricsScrapeWorkers), 1), 16)
	client := metrics.NewClient(lim.Timeout)
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, t := range targets {
		sem <- struct{}{}
		wg.Add(1)
		go func(t store.AppMetricsTarget) {
			defer func() { <-sem; wg.Done() }()
			msg := appScrapeMessage(rt.scrapeOneApp(ctx, db, client, t, lim, maxRows))
			if msg != t.LastError {
				_ = db.RecordAppMetricsScrapeError(ctx, t.App, msg)
			}
		}(t)
	}
	wg.Wait()
}

// appScrapeMessage is the fixed text stored as last_error; raw errors can carry the container address.
func appScrapeMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errAppContainerMissing):
		return "app container is not running"
	case errors.Is(err, store.ErrAppMetricsRowCap):
		return "stored sample limit reached, new samples dropped"
	case errors.Is(err, store.ErrServiceNotFound), errors.Is(err, store.ErrAppMetricsTargetNotFound):
		return "app or target was removed"
	default:
		return metrics.ErrorMessage(err)
	}
}

func (rt *Router) scrapeOneApp(ctx context.Context, db appMetricsStore, client *http.Client, t store.AppMetricsTarget, lim metrics.Limits, maxRows int) error {
	svc, err := rt.apps.GetDesiredService(ctx, t.App)
	if err != nil {
		return err
	}
	ip, err := rt.appContainerNetworkIP(ctx, *svc)
	if err != nil {
		return err
	}
	target, err := metrics.TargetURL(ip, t.Port, t.Path)
	if err != nil {
		return err
	}
	samples, _, err := metrics.Scrape(ctx, client, target, lim)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	rows := make([]store.AppMetricSample, len(samples))
	for i, s := range samples {
		rows[i] = store.AppMetricSample{Name: s.Name, Labels: s.Labels, At: now, Value: s.Value}
	}
	return db.InsertAppMetricSamples(ctx, t.App, rows, maxRows)
}

var errAppContainerMissing = errors.New("app container is not running")

// appContainerNetworkIP returns the app container's address on its own app network only.
func (rt *Router) appContainerNetworkIP(ctx context.Context, svc store.DesiredService) (string, error) {
	if rt.containers == nil {
		return "", errAppContainerMissing
	}
	want := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	list, err := rt.containers.ListByPrefix(ctx, want)
	if err != nil {
		return "", err
	}
	var hit *docker.ContainerState
	for i := range list {
		if list[i].Name == want && list[i].Running {
			hit = &list[i]
			break
		}
	}
	if hit == nil {
		return "", errAppContainerMissing
	}
	ip := sysEmailAppNetworkIP(*hit, svc.AppID)
	if ip == "" {
		return "", metrics.ErrBadTarget
	}
	return ip, nil
}
