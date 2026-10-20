package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/kit/forecast"
)

// defaultCapacityForecastLookback is how far back
// handleNodeCapacityForecast reads disk/memory history when
// WithCapacityForecastLookback isn't configured, wide enough (two
// weeks) that a short-lived usage bump doesn't dominate the fitted
// trend, narrow enough that a months-old growth spurt doesn't drag the
// line flat.
const defaultCapacityForecastLookback = 14 * 24 * time.Hour

// capacityForecastNote is returned on every response, not just attached
// to a warning: forecast.Package's own doc comment is the source of
// truth, this is the plain-English version a dashboard or CLI user
// actually reads.
const capacityForecastNote = "Rough projection from the recent usage trend, assuming it continues in a straight line. This is a heads-up, not a guarantee: real usage rarely grows this predictably."

// capacityForecastMetric mirrors forecast.Projection on the wire.
type capacityForecastMetric struct {
	CurrentUsedBytes float64   `json:"current_used_bytes"`
	TotalBytes       float64   `json:"total_bytes"`
	SlopeBytesPerDay float64   `json:"slope_bytes_per_day"`
	DaysUntilFull    float64   `json:"days_until_full"`
	ProjectedFullAt  time.Time `json:"projected_full_at"`
	SampleCount      int       `json:"sample_count"`
	CoverageWindow   string    `json:"coverage_window"`
}

func toCapacityForecastMetric(p *forecast.Projection) *capacityForecastMetric {
	if p == nil {
		return nil
	}
	return &capacityForecastMetric{
		CurrentUsedBytes: p.CurrentUsedBytes,
		TotalBytes:       p.TotalBytes,
		SlopeBytesPerDay: p.SlopeBytesPerDay,
		DaysUntilFull:    p.DaysUntilFull,
		ProjectedFullAt:  p.FullAt,
		SampleCount:      p.SampleCount,
		CoverageWindow:   p.Span.Round(time.Minute).String(),
	}
}

// nodeCapacityForecastResponse is GET
// /api/v1/nodes/{id}/capacity-forecast's wire shape. Disk/Memory are
// nil (omitted) whenever forecast.Project found no real growing trend
// or not enough history, not a zero-value projection: a caller must
// never treat an absent field as "0 days left."
type nodeCapacityForecastResponse struct {
	NodeID         string                  `json:"node_id"`
	LookbackWindow string                  `json:"lookback_window"`
	Disk           *capacityForecastMetric `json:"disk,omitempty"`
	Memory         *capacityForecastMetric `json:"memory,omitempty"`
	Note           string                  `json:"note"`
}

// handleNodeCapacityForecast handles GET
// /api/v1/nodes/{id}/capacity-forecast: a rough "days until full"
// projection for this node's disk and memory, from internal/forecast's
// trend fit over the host samples HostDiskCollector/HostMemoryCollector
// already write. Read-and-suggest only, like handleAppResourceRecommendation.
func (rt *Router) handleNodeCapacityForecast(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: node capacity forecast: load node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	lookback := rt.capacityForecastLookback
	if lookback <= 0 {
		lookback = defaultCapacityForecastLookback
	}

	now := time.Now()
	from := now.Add(-lookback)
	resourceID := nodeResourceID(id)

	writeJSON(w, http.StatusOK, nodeCapacityForecastResponse{
		NodeID:         id,
		LookbackWindow: lookback.String(),
		Disk:           toCapacityForecastMetric(rt.diskCapacityForecast(r.Context(), id, resourceID, from, now)),
		Memory:         toCapacityForecastMetric(rt.memoryCapacityForecast(r.Context(), id, resourceID, from, now)),
		Note:           capacityForecastNote,
	})
}

// diskCapacityForecast reads disk_used_bytes/disk_total_bytes under
// resourceID and fits a trend. A missing series (never collected, or no
// samples in the lookback window) just means "nothing to project," the
// same best-effort shape writeNodeHostMetric already treats a query
// failure with.
func (rt *Router) diskCapacityForecast(ctx context.Context, nodeID, resourceID string, from, to time.Time) *forecast.Projection {
	used, err := rt.telemetry.QueryMetrics(ctx, resourceID, telemetry.MetricDiskUsedBytes, from, to)
	if err != nil {
		rt.logger.Warn("api: node capacity forecast: query disk used failed", slog.String("error", err.Error()), slog.String("node_id", nodeID))
		return nil
	}
	if len(used) == 0 {
		return nil
	}
	total, err := rt.telemetry.QueryMetrics(ctx, resourceID, telemetry.MetricDiskTotalBytes, from, to)
	if err != nil {
		rt.logger.Warn("api: node capacity forecast: query disk total failed", slog.String("error", err.Error()), slog.String("node_id", nodeID))
		return nil
	}
	if len(total) == 0 {
		return nil
	}

	points := make([]forecast.Point, len(used))
	for i, s := range used {
		points[i] = forecast.Point{Timestamp: s.Timestamp, UsedBytes: s.Value}
	}
	return forecast.Project(points, total[len(total)-1].Value, to)
}

// memoryCapacityForecast reads memory_total_bytes/memory_available_bytes
// under resourceID, turns them into used-bytes points (total minus
// available, joined by their shared collection timestamp since
// HostMemoryCollector.CollectOnce stamps both samples with the same
// `now` on every tick), and fits a trend.
func (rt *Router) memoryCapacityForecast(ctx context.Context, nodeID, resourceID string, from, to time.Time) *forecast.Projection {
	totalSamples, err := rt.telemetry.QueryMetrics(ctx, resourceID, telemetry.MetricMemoryTotalBytes, from, to)
	if err != nil {
		rt.logger.Warn("api: node capacity forecast: query memory total failed", slog.String("error", err.Error()), slog.String("node_id", nodeID))
		return nil
	}
	if len(totalSamples) == 0 {
		return nil
	}
	availSamples, err := rt.telemetry.QueryMetrics(ctx, resourceID, telemetry.MetricMemoryAvailableBytes, from, to)
	if err != nil {
		rt.logger.Warn("api: node capacity forecast: query memory available failed", slog.String("error", err.Error()), slog.String("node_id", nodeID))
		return nil
	}
	if len(availSamples) == 0 {
		return nil
	}

	totalByTimestamp := make(map[int64]float64, len(totalSamples))
	for _, s := range totalSamples {
		totalByTimestamp[s.Timestamp.Unix()] = s.Value
	}

	points := make([]forecast.Point, 0, len(availSamples))
	for _, s := range availSamples {
		total, ok := totalByTimestamp[s.Timestamp.Unix()]
		if !ok {
			continue // a tick where only one of the pair landed; skip rather than guess
		}
		points = append(points, forecast.Point{Timestamp: s.Timestamp, UsedBytes: total - s.Value})
	}

	return forecast.Project(points, totalSamples[len(totalSamples)-1].Value, to)
}
