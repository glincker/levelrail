package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/diskhistory"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// DiskHistoryReader is the store surface GET /api/v1/system/disk/history needs.
type DiskHistoryReader interface {
	Since(ctx context.Context, since time.Time) ([]diskhistory.Sample, error)
}

// SetDiskHistory enables /api/v1/system/disk/history; without it the route answers 501.
func (rt *Router) SetDiskHistory(r DiskHistoryReader) { rt.diskHistory = r }

// DiskHistorySource gives the sampler live readings: data dir statfs and the cached Docker usage.
func (rt *Router) DiskHistorySource() diskhistory.Source {
	src := diskhistory.Source{Stat: func() (int64, int64, error) { return diskguard.SystemStat(rt.dataDir) }}
	if rt.dockerDiskUsage != nil {
		src.Usage = func(ctx context.Context) (diskhistory.Categories, error) {
			u, err := rt.dockerDiskUsage.DiskUsage(ctx)
			if err != nil {
				return diskhistory.Categories{}, err
			}
			return diskhistory.Categories{
				Images: u.ImagesTotalBytes, Containers: u.ContainersTotalBytes,
				Volumes: u.VolumesTotalBytes, BuildCache: u.BuildCacheTotalBytes,
			}, nil
		}
	}
	return src
}

type diskForecastResource struct {
	DaysUntilFull *float64 `json:"days_until_full"`
	BasisDays     int      `json:"basis_days"`
}

type diskHistoryResponse struct {
	Samples       []diskhistory.Sample `json:"samples"`
	Forecast      diskForecastResource `json:"forecast"`
	RetentionDays int                  `json:"retention_days"`
}

// handleDiskHistory handles GET /api/v1/system/disk/history?days=N. N defaults to and is capped at the retention setting.
func (rt *Router) handleDiskHistory(w http.ResponseWriter, r *http.Request) {
	if rt.diskHistory == nil {
		writeError(w, http.StatusNotImplemented, "disk history is not enabled")
		return
	}
	retention := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDiskHistoryRetentionDays)
	days := retention
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "days must be a positive integer")
			return
		}
		days = min(n, retention)
	}
	now := time.Now()
	samples, err := rt.diskHistory.Since(r.Context(), now.Add(-time.Duration(days)*24*time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read disk history")
		return
	}
	if samples == nil {
		samples = []diskhistory.Sample{}
	}
	resp := diskHistoryResponse{Samples: samples, RetentionDays: retention, Forecast: diskForecastResource{BasisDays: diskhistory.BasisDays(samples)}}
	// The forecast fits the configured window, whatever range the chart asked for.
	window := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDiskForecastWindowDays)
	minDays := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDiskForecastMinDays)
	fit := samples
	if days < window {
		if fit, err = rt.diskHistory.Since(r.Context(), now.Add(-time.Duration(window)*24*time.Hour)); err != nil {
			fit = samples
		}
	}
	if f := diskhistory.ForecastWith(fit, window, minDays); f != nil {
		resp.Forecast.DaysUntilFull = &f.DaysUntilFull
		resp.Forecast.BasisDays = f.BasisDays
	}
	writeJSON(w, http.StatusOK, resp)
}
