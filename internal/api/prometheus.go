package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang/snappy"

	"github.com/GLINCKER/levelrail/internal/prompb"
)

// levelrailMetricPrefix namespaces every metric this control plane
// exposes over remote-read, so "cpu_percent" (internal/telemetry's own
// metric name) becomes "levelrail_cpu_percent" in Prometheus's flat,
// global metric-name space, per CLAUDE.md 4.8: "expose a
// Prometheus-compatible remote read endpoint so an operator who wants
// Grafana anyway can point it at Levelrail." Prefixing is the standard
// Prometheus convention for avoiding collisions with other exporters
// the same Prometheus/Grafana instance might also be reading from.
const levelrailMetricPrefix = "levelrail_"

// serviceLabel is the one label (beyond __name__) this endpoint
// supports matching on: which app's metrics a query wants. Matches
// resourceIDForApp's "service:" prefix convention used elsewhere in
// this package (metrics.go, logs.go).
const serviceLabel = "service"

// handlePrometheusRead handles POST /api/v1/prometheus/read, the
// address an operator configures in Prometheus's own remote_read.url
// (or Grafana's Prometheus data source pointed directly at Levelrail,
// per CLAUDE.md 4.8). Request and response bodies are snappy-compressed
// protobuf (internal/prompb), the standard Prometheus remote-read
// transport. Gated by requireAbility(AbilityRead, ...) in router.go,
// the same as every other read route: an operator configures
// Prometheus's remote_read.authorization.credentials with a
// Levelrail API token scoped to at least "read" (see the token
// management routes above), since leaving a metrics endpoint
// unauthenticated would let any caller pull every service's resource
// usage.
//
// Query support is deliberately narrow for this pass: exactly one EQ
// matcher on __name__ (stripped of the levelrail_ prefix to get the
// internal/telemetry metric name) and exactly one EQ matcher on
// service (mapped to resourceIDForApp) per Query. Anything broader
// (regex matchers, multiple services in one query, an __name__ this
// control plane doesn't expose) returns an empty result for that
// specific Query rather than an error: Prometheus's own query engine
// treats "no data for this selector" as a normal, common outcome, not
// a failure, and erroring here would make every dashboard panel using
// a selector this endpoint can't yet satisfy show as broken instead of
// simply empty. Broader matcher support (regex, multi-series) is a
// real, explicit follow-up, not implied by this comment.
func (rt *Router) handlePrometheusRead(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	compressed, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	raw, err := snappy.Decode(nil, compressed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid snappy encoding")
		return
	}
	req, err := prompb.UnmarshalReadRequest(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid protobuf ReadRequest")
		return
	}

	resp := prompb.ReadResponse{Results: make([]prompb.QueryResult, len(req.Queries))}
	for i, q := range req.Queries {
		resp.Results[i] = rt.answerPromQuery(r.Context(), q)
	}

	body := snappy.Encode(nil, prompb.MarshalReadResponse(resp))
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Encoding", "snappy")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		rt.logger.Error("api: prometheus read: write response failed", slog.String("error", err.Error()))
	}
}

// answerPromQuery resolves one prompb.Query into a QueryResult. Never
// returns an error itself: an unsatisfiable or partially-failed query
// becomes an empty (or partial) result, logged, matching this
// endpoint's own "empty is normal" contract documented on
// handlePrometheusRead.
func (rt *Router) answerPromQuery(ctx context.Context, q prompb.Query) prompb.QueryResult {
	metric, service, ok := matchersToMetricAndService(q.Matchers)
	if !ok {
		return prompb.QueryResult{}
	}

	from := time.UnixMilli(q.StartTimestampMs).UTC()
	to := time.UnixMilli(q.EndTimestampMs).UTC()

	samples, err := rt.telemetry.QueryMetrics(ctx, resourceIDForApp(service), metric, from, to)
	if err != nil && len(samples) == 0 {
		rt.logger.Warn("api: prometheus read: query failed",
			slog.String("error", err.Error()), slog.String("metric", metric), slog.String("service", service))
		return prompb.QueryResult{}
	}
	if len(samples) == 0 {
		return prompb.QueryResult{}
	}

	promSamples := make([]prompb.Sample, len(samples))
	for i, s := range samples {
		promSamples[i] = prompb.Sample{Value: s.Value, Timestamp: s.Timestamp.UnixMilli()}
	}

	return prompb.QueryResult{
		TimeSeries: []prompb.TimeSeries{{
			Labels: []prompb.Label{
				{Name: "__name__", Value: levelrailMetricPrefix + metric},
				{Name: serviceLabel, Value: service},
			},
			Samples: promSamples,
		}},
	}
}

// matchersToMetricAndService extracts the one metric name and one
// service name a Query's matchers identify, per handlePrometheusRead's
// documented narrow-matcher-support contract: exactly one EQ matcher on
// __name__ (levelrail_-prefixed) and exactly one EQ matcher on service,
// nothing else. ok is false for anything broader, telling the caller to
// answer with an empty result rather than guessing at partial matches.
func matchersToMetricAndService(matchers []prompb.LabelMatcher) (metric, service string, ok bool) {
	for _, m := range matchers {
		if m.Type != prompb.MatchEqual {
			return "", "", false
		}
		switch m.Name {
		case "__name__":
			if metric != "" || !strings.HasPrefix(m.Value, levelrailMetricPrefix) {
				return "", "", false
			}
			metric = strings.TrimPrefix(m.Value, levelrailMetricPrefix)
		case serviceLabel:
			if service != "" {
				return "", "", false
			}
			service = m.Value
		default:
			return "", "", false
		}
	}
	if metric == "" || service == "" {
		return "", "", false
	}
	return metric, service, true
}
