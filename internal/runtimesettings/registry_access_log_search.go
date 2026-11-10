package runtimesettings

import (
	"context"
	"time"
)

// Access log search bounds. The rows live in the edge's in-memory ring, so the
// scan cap is what keeps one tenant's search from holding CPU on a busy edge.
const (
	KeyAccessSearchMaxWindow = "telemetry.access_search_max_window"
	KeyAccessSearchMaxRows   = "telemetry.access_search_max_rows"
	KeyAccessSearchMaxTerm   = "telemetry.access_search_max_term"
	KeyAccessSearchMaxScan   = "telemetry.access_search_max_scan"
	KeyAccessSearchMaxConc   = "telemetry.access_search_max_concurrent"
)

// AccessSearchLimits bounds one access log search, read fresh on every request.
type AccessSearchLimits struct {
	MaxWindow time.Duration
	MaxRows   int
	MaxTerm   int
	MaxScan   int
	MaxConc   int
}

func init() {
	Register(Definition{
		Key: KeyAccessSearchMaxWindow, Category: CategoryObservability, Kind: KindDuration, Default: "24h", Env: "APP_ACCESS_SEARCH_MAX_WINDOW", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Minute, 720*time.Hour),
		Title:      "Access log search: longest window", Description: "Longest time range one access log search may cover, from 1m to 720h. Rows older than the in-memory log keeps are gone either way.",
	})
	registerLimitInt(KeyAccessSearchMaxRows, "APP_ACCESS_SEARCH_MAX_ROWS", "200", "Access log search: max rows per page", "Rows one access log search page may return, from 1 to 5000; larger requests are clamped.", 1, 5000)
	registerLimitInt(KeyAccessSearchMaxTerm, "APP_ACCESS_SEARCH_MAX_TERM", "256", "Access log search: max term length", "Longest path prefix or search text accepted, from 1 to 4096 bytes.", 1, 4096)
	registerLimitInt(KeyAccessSearchMaxConc, "APP_ACCESS_SEARCH_MAX_CONCURRENT", "4", "Access log search: concurrent searches", "Searches that may run at once on this control plane, from 1 to 32; more answer 429.", 1, 32)
	registerLimitInt(KeyAccessSearchMaxScan, "APP_ACCESS_SEARCH_MAX_SCAN", "200000", "Access log search: max rows scanned", "Records one search may examine before it stops and reports a partial result, from 1000 to 10000000.", 1000, 10_000_000)
}

// AccessSearchLimits returns the live access log search bounds; a nil service serves env then defaults.
func (s *Service) AccessSearchLimits(ctx context.Context) AccessSearchLimits {
	return AccessSearchLimits{
		MaxWindow: s.Duration(ctx, KeyAccessSearchMaxWindow),
		MaxRows:   s.Int(ctx, KeyAccessSearchMaxRows),
		MaxTerm:   s.Int(ctx, KeyAccessSearchMaxTerm),
		MaxScan:   s.Int(ctx, KeyAccessSearchMaxScan),
		MaxConc:   s.Int(ctx, KeyAccessSearchMaxConc),
	}
}
