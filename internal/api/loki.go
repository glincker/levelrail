package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	lokiBasePath         = "/api/v1/loki"
	lokiGrafanaBasePath  = lokiBasePath + "/loki/api/v1"
	lokiDefaultLimit     = 100
	lokiMaxLimit         = 5000
	lokiInstantWindow    = 5 * time.Minute
	lokiDefaultRange     = time.Hour
	defaultLokiMaxRange  = 30 * 24 * time.Hour
	envLokiMaxRange      = "APP_LOKI_MAX_RANGE"
	lokiMaxAppsPerQuery  = 100
	lokiResultTypeStream = "streams"
)

func lokiMaxRange() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envLokiMaxRange)); err == nil && d > 0 {
		return d
	}
	return defaultLokiMaxRange
}

type lokiError struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

func writeLokiError(w http.ResponseWriter, code int, errType, msg string) {
	writeJSON(w, code, lokiError{Status: "error", ErrorType: errType, Error: msg})
}

// lokiBasicToBearer lets Grafana's Basic auth password carry an API token:
// requireAbility only reads Bearer, so the password is moved there first.
func lokiBasicToBearer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "basic ") {
			if _, pass, ok := r.BasicAuth(); ok && pass != "" {
				r.Header.Set("Authorization", "Bearer "+pass)
			}
		}
		next(w, r)
	}
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiStreamsData struct {
	ResultType string       `json:"resultType"`
	Result     []lokiStream `json:"result"`
}

type lokiQueryResponse struct {
	Status string          `json:"status"`
	Data   lokiStreamsData `json:"data"`
}

// parseLokiTime accepts unix seconds (float ok), unix nanoseconds or RFC3339.
func parseLokiTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 {
		return time.Time{}, errors.New("time must be unix seconds, unix nanoseconds or RFC3339")
	}
	if f >= 1e17 {
		n, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil {
			n = int64(f)
		}
		return time.Unix(0, n), nil
	}
	return time.Unix(0, int64(f*1e9)), nil
}

func (rt *Router) handleLokiQueryRange(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			q = r.Form
		}
	}
	now := time.Now()
	to, from := now, now.Add(-lokiDefaultRange)
	var err error
	if raw := q.Get("end"); raw != "" {
		if to, err = parseLokiTime(raw); err != nil {
			writeLokiError(w, http.StatusBadRequest, "bad_data", "invalid end: "+err.Error())
			return
		}
	}
	if raw := q.Get("start"); raw != "" {
		if from, err = parseLokiTime(raw); err != nil {
			writeLokiError(w, http.StatusBadRequest, "bad_data", "invalid start: "+err.Error())
			return
		}
	} else if q.Get("end") != "" {
		from = to.Add(-lokiDefaultRange)
	}
	rt.serveLokiLogs(w, r, q.Get("query"), q.Get("limit"), q.Get("direction"), from, to)
}

func (rt *Router) handleLokiQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			q = r.Form
		}
	}
	at := time.Now()
	if raw := q.Get("time"); raw != "" {
		t, err := parseLokiTime(raw)
		if err != nil {
			writeLokiError(w, http.StatusBadRequest, "bad_data", "invalid time: "+err.Error())
			return
		}
		at = t
	}
	rt.serveLokiLogs(w, r, q.Get("query"), q.Get("limit"), q.Get("direction"), at.Add(-lokiInstantWindow), at)
}

func (rt *Router) serveLokiLogs(w http.ResponseWriter, r *http.Request, query, limitRaw, direction string, from, to time.Time) {
	if rt.telemetry == nil {
		writeLokiError(w, http.StatusNotImplemented, "unavailable", "telemetry is not configured on this control plane")
		return
	}
	parsed, err := telemetry.ParseLogQL(query)
	if err != nil {
		writeLokiError(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	limit := lokiDefaultLimit
	if limitRaw != "" {
		n, err := strconv.Atoi(limitRaw)
		if err != nil || n < 1 {
			writeLokiError(w, http.StatusBadRequest, "bad_data", "limit must be a positive integer")
			return
		}
		limit = min(n, lokiMaxLimit)
	}
	forward := false
	switch strings.ToLower(direction) {
	case "", "backward":
	case "forward":
		forward = true
	default:
		writeLokiError(w, http.StatusBadRequest, "bad_data", "direction must be forward or backward")
		return
	}
	if from.After(to) {
		writeLokiError(w, http.StatusBadRequest, "bad_data", "start must not be after end")
		return
	}
	if maxRange := lokiMaxRange(); to.Sub(from) > maxRange {
		writeLokiError(w, http.StatusBadRequest, "bad_data", "time range is longer than the allowed "+maxRange.String())
		return
	}

	apps, err := rt.lokiVisibleApps(r, parsed.MatchApp)
	if err != nil {
		rt.internalError(w, "api: loki: list apps", err)
		return
	}
	if len(apps) > lokiMaxAppsPerQuery {
		writeLokiError(w, http.StatusBadRequest, "bad_data",
			"selector matches "+strconv.Itoa(len(apps))+" apps, narrow it to at most "+strconv.Itoa(lokiMaxAppsPerQuery))
		return
	}

	type hit struct {
		app string
		e   telemetry.LogEntry
		lvl string
	}
	maxRows := logSignatureMaxRows()
	var hits []hit
	for _, app := range apps {
		entries, _, err := telemetry.QueryLogsBounded(r.Context(), rt.telemetry, resourceIDForApp(app), from, to, "", maxRows)
		if err != nil && len(entries) == 0 {
			rt.logger.Warn("api: loki: query failed", slog.String("app", app), slog.String("error", err.Error()))
			continue
		}
		for _, e := range entries {
			lvl := telemetry.ClassifyLevel(e)
			labels := map[string]string{
				telemetry.LogQLLabelApp: app, telemetry.LogQLLabelStream: e.Stream, telemetry.LogQLLabelLevel: lvl,
			}
			if parsed.MatchLabels(labels) && parsed.MatchLine(e.Message) {
				hits = append(hits, hit{app: app, e: e, lvl: lvl})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if forward {
			return hits[i].e.Timestamp.Before(hits[j].e.Timestamp)
		}
		return hits[i].e.Timestamp.After(hits[j].e.Timestamp)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}

	order := []string{}
	byKey := map[string]*lokiStream{}
	for _, h := range hits {
		labels := map[string]string{telemetry.LogQLLabelApp: h.app, telemetry.LogQLLabelStream: h.e.Stream}
		if h.lvl != "" {
			labels[telemetry.LogQLLabelLevel] = h.lvl
		}
		key := h.app + "\x00" + h.e.Stream + "\x00" + h.lvl
		s, ok := byKey[key]
		if !ok {
			s = &lokiStream{Stream: labels, Values: [][2]string{}}
			byKey[key] = s
			order = append(order, key)
		}
		s.Values = append(s.Values, [2]string{strconv.FormatInt(h.e.Timestamp.UnixNano(), 10), h.e.Message})
	}
	sort.Strings(order)
	result := make([]lokiStream, 0, len(order))
	for _, k := range order {
		result = append(result, *byKey[k])
	}
	writeJSON(w, http.StatusOK, lokiQueryResponse{Status: "success", Data: lokiStreamsData{ResultType: lokiResultTypeStream, Result: result}})
}

// lokiVisibleApps returns the sorted names the caller may read that pass keep.
func (rt *Router) lokiVisibleApps(r *http.Request, keep func(string) bool) ([]string, error) {
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		return nil, err
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(svcs))
	for _, s := range svcs {
		if canSee(s.Name) && keep(s.Name) {
			names = append(names, s.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func writeLokiList(w http.ResponseWriter, values []string) {
	if values == nil {
		values = []string{}
	}
	writeJSON(w, http.StatusOK, struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}{Status: "success", Data: values})
}

func (rt *Router) handleLokiLabels(w http.ResponseWriter, _ *http.Request) {
	writeLokiList(w, []string{telemetry.LogQLLabelApp, telemetry.LogQLLabelLevel, telemetry.LogQLLabelStream})
}

func (rt *Router) handleLokiLabelValues(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("name") {
	case telemetry.LogQLLabelApp:
		apps, err := rt.lokiVisibleApps(r, func(string) bool { return true })
		if err != nil {
			rt.internalError(w, "api: loki: label values", err)
			return
		}
		writeLokiList(w, apps)
	case telemetry.LogQLLabelStream:
		writeLokiList(w, []string{"stderr", "stdout"})
	case telemetry.LogQLLabelLevel:
		writeLokiList(w, []string{
			telemetry.LevelTrace, telemetry.LevelDebug, telemetry.LevelInfo,
			telemetry.LevelWarn, telemetry.LevelError, telemetry.LevelFatal,
		})
	default:
		writeLokiList(w, nil)
	}
}

func (rt *Router) handleLokiReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ready\n"))
}

// registerLokiRoutes serves the read API under both the short base and the
// /loki/api/v1 suffix Grafana appends to the datasource URL.
func (rt *Router) registerLokiRoutes(mux *http.ServeMux) {
	for _, base := range []string{lokiBasePath, lokiGrafanaBasePath} {
		wrap := func(h http.HandlerFunc) http.HandlerFunc {
			return lokiBasicToBearer(rt.requireAbility(AbilityRead, h))
		}
		mux.HandleFunc("GET "+base+"/query_range", wrap(rt.handleLokiQueryRange))
		mux.HandleFunc("POST "+base+"/query_range", wrap(rt.handleLokiQueryRange))
		mux.HandleFunc("GET "+base+"/query", wrap(rt.handleLokiQuery))
		mux.HandleFunc("POST "+base+"/query", wrap(rt.handleLokiQuery))
		mux.HandleFunc("GET "+base+"/labels", wrap(rt.handleLokiLabels))
		mux.HandleFunc("GET "+base+"/label/{name}/values", wrap(rt.handleLokiLabelValues))
	}
	mux.HandleFunc("GET "+lokiBasePath+"/ready", lokiBasicToBearer(rt.requireAbility(AbilityRead, rt.handleLokiReady)))
}
