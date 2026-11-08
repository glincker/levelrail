package apiclient

import (
	"context"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/diskhistory"
)

// DiskForecast mirrors the forecast block of GET /api/v1/system/disk/history.
type DiskForecast struct {
	DaysUntilFull *float64 `json:"days_until_full"`
	BasisDays     int      `json:"basis_days"`
}

// DiskHistory mirrors GET /api/v1/system/disk/history.
type DiskHistory struct {
	Samples       []diskhistory.Sample `json:"samples"`
	Forecast      DiskForecast         `json:"forecast"`
	RetentionDays int                  `json:"retention_days"`
}

// GetDiskHistory calls GET /api/v1/system/disk/history; days 0 lets the server pick its retention window.
func (c *Client) GetDiskHistory(ctx context.Context, days int) (DiskHistory, error) {
	path := "/api/v1/system/disk/history"
	if days > 0 {
		path += "?days=" + strconv.Itoa(days)
	}
	var out DiskHistory
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
