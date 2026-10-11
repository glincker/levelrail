package telemetry

import (
	"os"
	"strconv"
	"time"
)

// EnvMaxPoints names the env var that caps points per series.
const EnvMaxPoints = "APP_METRICS_MAX_POINTS"

// DefaultMaxPoints is the series cap when EnvMaxPoints is unset.
const DefaultMaxPoints = 600

// MaxPointsHardCap bounds what a caller may ask for.
const MaxPointsHardCap = 5000

// MaxPointsFromEnv returns the configured per-series point cap.
func MaxPointsFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv(EnvMaxPoints)); err == nil && n > 0 {
		return min(n, MaxPointsHardCap)
	}
	return DefaultMaxPoints
}

var niceSteps = []time.Duration{
	15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute,
	10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour,
	6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

// AutoStep picks the smallest round step, no finer than floor, that keeps a
// series over span within maxPoints. A non-positive maxPoints or span yields
// floor.
func AutoStep(span time.Duration, maxPoints int, floor time.Duration) time.Duration {
	if floor <= 0 {
		floor = MinRequestStep
	}
	if span <= 0 || maxPoints <= 0 {
		return floor
	}
	need := span / time.Duration(maxPoints)
	for _, s := range niceSteps {
		if s >= floor && s >= need {
			return s
		}
	}
	return niceSteps[len(niceSteps)-1]
}

// ShiftPoints returns pts with every timestamp moved forward by d, so a
// previous-period series lines up with the current window.
func ShiftPoints(pts []AggregatedPoint, d time.Duration) []AggregatedPoint {
	out := make([]AggregatedPoint, len(pts))
	for i, p := range pts {
		p.Timestamp = p.Timestamp.Add(d)
		out[i] = p
	}
	return out
}
