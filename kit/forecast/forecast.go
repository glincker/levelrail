// Package forecast projects when a resource such as a disk will fill, using an
// ordinary least-squares line fit over (timestamp, bytes used) samples. Real
// usage rarely grows linearly, so the result is a rough estimate.
package forecast

import (
	"sort"
	"time"
)

// minPoints and minSpan gate whether there is enough history to fit a
// trend at all. Two samples a minute apart "trend toward full" exactly
// as confidently, and as meaninglessly, as thirty samples spread across
// a week; both the point count and the time span must clear a floor.
const (
	minPoints = 6
	minSpan   = 6 * time.Hour
)

// Point is one (timestamp, bytes used) observation. Timestamps need not
// be sorted or evenly spaced; Project sorts its own copy.
type Point struct {
	Timestamp time.Time
	UsedBytes float64
}

// Projection is Project's result for one resource: a real, growing
// trend with a projected exhaustion point. Project returns nil instead
// of a Projection when the trend is flat, improving, or under-sampled,
// so a nil result always means "nothing to warn about," never "zero
// days left."
type Projection struct {
	// SlopeBytesPerDay is the fitted line's slope: always > 0, since
	// Project returns nil otherwise.
	SlopeBytesPerDay float64
	// DaysUntilFull is how many days from now the fitted line crosses
	// TotalBytes, floored at 0 (never negative, even if the line has
	// already crossed).
	DaysUntilFull float64
	// FullAt is now plus DaysUntilFull, the projected exhaustion
	// timestamp.
	FullAt time.Time
	// CurrentUsedBytes is the fitted line's value at now, not the raw
	// last sample, so a single noisy spike doesn't move the headline
	// number.
	CurrentUsedBytes float64
	TotalBytes       float64
	SampleCount      int
	// Span is the time between the earliest and latest sample Project
	// fit the line over.
	Span time.Duration
}

// Project fits an ordinary-least-squares line through points and
// projects it forward to totalBytes. Returns nil when there isn't
// enough history (fewer than minPoints samples, or less than minSpan
// between the earliest and latest), when totalBytes isn't a real
// capacity figure (<= 0), or when the fitted slope is zero or negative:
// an operator should never be warned about a trend that isn't actually
// pointing at exhaustion, even if the raw samples are noisy around a
// flat average.
func Project(points []Point, totalBytes float64, now time.Time) *Projection {
	if len(points) < minPoints || totalBytes <= 0 {
		return nil
	}

	sorted := make([]Point, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp.Before(sorted[j].Timestamp) })

	span := sorted[len(sorted)-1].Timestamp.Sub(sorted[0].Timestamp)
	if span < minSpan {
		return nil
	}

	slope, intercept := leastSquaresPerDay(sorted)
	if slope <= 0 {
		return nil
	}

	t0 := sorted[0].Timestamp
	tNow := now.Sub(t0).Hours() / 24
	fittedNow := intercept + slope*tNow

	remaining := totalBytes - fittedNow
	daysUntilFull := remaining / slope
	if daysUntilFull < 0 {
		daysUntilFull = 0
	}

	return &Projection{
		SlopeBytesPerDay: slope,
		DaysUntilFull:    daysUntilFull,
		FullAt:           now.Add(time.Duration(daysUntilFull * 24 * float64(time.Hour))),
		CurrentUsedBytes: fittedNow,
		TotalBytes:       totalBytes,
		SampleCount:      len(sorted),
		Span:             span,
	}
}

// leastSquaresPerDay fits y = intercept + slope*x over points, with x
// measured in days since points[0].Timestamp (points must already be
// sorted ascending by Timestamp, and non-empty). slope's unit is
// therefore bytes per day.
func leastSquaresPerDay(points []Point) (slope, intercept float64) {
	t0 := points[0].Timestamp
	n := float64(len(points))

	var sumX, sumY, sumXY, sumXX float64
	for _, p := range points {
		x := p.Timestamp.Sub(t0).Hours() / 24
		y := p.UsedBytes
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		// Every point shares the same x (shouldn't happen once span >=
		// minSpan has been checked, but stay defined): no slope, just
		// the mean.
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return slope, intercept
}
