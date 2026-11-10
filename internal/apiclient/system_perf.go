package apiclient

import (
	"context"
	"net/http"
	"time"
)

// SlowStatement is one entry of the control plane's slowest statements ring.
type SlowStatement struct {
	SQL        string    `json:"sql"`
	MaxMS      float64   `json:"max_ms"`
	LastMS     float64   `json:"last_ms"`
	Count      int64     `json:"count"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// SlowStatements is GET /api/v1/system/slow-statements.
type SlowStatements struct {
	Enabled         bool            `json:"enabled"`
	SlowThresholdMS int             `json:"slow_threshold_ms"`
	RingSize        int             `json:"ring_size"`
	Statements      []SlowStatement `json:"statements"`
}

// GetSlowStatements calls GET /api/v1/system/slow-statements.
func (c *Client) GetSlowStatements(ctx context.Context) (SlowStatements, error) {
	var out SlowStatements
	err := c.do(ctx, http.MethodGet, "/api/v1/system/slow-statements", nil, &out)
	return out, err
}
