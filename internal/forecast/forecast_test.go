package forecast

import (
	"testing"
	"time"
)

func daily(start time.Time, usedByDay []float64) []Point {
	points := make([]Point, len(usedByDay))
	for i, used := range usedByDay {
		points[i] = Point{Timestamp: start.Add(time.Duration(i) * 24 * time.Hour), UsedBytes: used}
	}
	return points
}

func TestProject(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		points        []Point
		totalBytes    float64
		now           time.Time
		wantNil       bool
		wantDays      float64
		wantDaysDelta float64
	}{
		{
			name:       "steady growth projects exact days to full",
			points:     daily(start, []float64{1000, 1100, 1200, 1300, 1400, 1500, 1600, 1700, 1800, 1900}),
			totalBytes: 2000,
			// day 9 (last sample) is the latest timestamp; now = that
			// timestamp, fitted value there is exactly 1900, slope is
			// exactly 100/day, so remaining 100 bytes / 100 per day = 1.
			now:           start.Add(9 * 24 * time.Hour),
			wantNil:       false,
			wantDays:      1.0,
			wantDaysDelta: 0.001,
		},
		{
			name:       "flat usage returns no warning",
			points:     daily(start, []float64{500, 500, 500, 500, 500, 500, 500, 500}),
			totalBytes: 2000,
			now:        start.Add(7 * 24 * time.Hour),
			wantNil:    true,
		},
		{
			name:       "improving (shrinking) usage returns no warning",
			points:     daily(start, []float64{900, 850, 800, 750, 700, 650, 600, 550}),
			totalBytes: 2000,
			now:        start.Add(7 * 24 * time.Hour),
			wantNil:    true,
		},
		{
			name:       "too few points returns no warning even with a clear trend",
			points:     daily(start, []float64{1000, 1100, 1200, 1300}),
			totalBytes: 2000,
			now:        start.Add(3 * 24 * time.Hour),
			wantNil:    true,
		},
		{
			name: "too short a span returns no warning even with enough points",
			points: []Point{
				{Timestamp: start, UsedBytes: 1000},
				{Timestamp: start.Add(1 * time.Minute), UsedBytes: 1010},
				{Timestamp: start.Add(2 * time.Minute), UsedBytes: 1020},
				{Timestamp: start.Add(3 * time.Minute), UsedBytes: 1030},
				{Timestamp: start.Add(4 * time.Minute), UsedBytes: 1040},
				{Timestamp: start.Add(5 * time.Minute), UsedBytes: 1050},
			},
			totalBytes: 2000,
			now:        start.Add(5 * time.Minute),
			wantNil:    true,
		},
		{
			name:       "zero or negative total capacity returns no warning",
			points:     daily(start, []float64{1000, 1100, 1200, 1300, 1400, 1500, 1600}),
			totalBytes: 0,
			now:        start.Add(6 * 24 * time.Hour),
			wantNil:    true,
		},
		{
			name:          "already past total still returns a non-negative days figure",
			points:        daily(start, []float64{1000, 1200, 1400, 1600, 1800, 2000, 2200, 2400}),
			totalBytes:    2000,
			now:           start.Add(7 * 24 * time.Hour),
			wantNil:       false,
			wantDays:      0,
			wantDaysDelta: 0.001,
		},
		{
			name:          "out of order input is sorted before fitting",
			points:        reversed(daily(start, []float64{1000, 1100, 1200, 1300, 1400, 1500, 1600, 1700, 1800, 1900})),
			totalBytes:    2000,
			now:           start.Add(9 * 24 * time.Hour),
			wantNil:       false,
			wantDays:      1.0,
			wantDaysDelta: 0.001,
		},
		{
			name: "unevenly spaced points still fit a consistent slope",
			points: []Point{
				{Timestamp: start, UsedBytes: 1000},
				{Timestamp: start.Add(1 * 24 * time.Hour), UsedBytes: 1100},
				{Timestamp: start.Add(3 * 24 * time.Hour), UsedBytes: 1300},
				{Timestamp: start.Add(6 * 24 * time.Hour), UsedBytes: 1600},
				{Timestamp: start.Add(8 * 24 * time.Hour), UsedBytes: 1800},
				{Timestamp: start.Add(10 * 24 * time.Hour), UsedBytes: 2000},
			},
			totalBytes:    2200,
			now:           start.Add(10 * 24 * time.Hour),
			wantNil:       false,
			wantDays:      2.0,
			wantDaysDelta: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Project(tt.points, tt.totalBytes, tt.now)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("Project() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Project() = nil, want a projection with DaysUntilFull ~ %v", tt.wantDays)
			}
			if diff := got.DaysUntilFull - tt.wantDays; diff > tt.wantDaysDelta || diff < -tt.wantDaysDelta {
				t.Errorf("DaysUntilFull = %v, want %v (+/- %v)", got.DaysUntilFull, tt.wantDays, tt.wantDaysDelta)
			}
			if got.SlopeBytesPerDay <= 0 {
				t.Errorf("SlopeBytesPerDay = %v, want > 0", got.SlopeBytesPerDay)
			}
			if got.TotalBytes != tt.totalBytes {
				t.Errorf("TotalBytes = %v, want %v", got.TotalBytes, tt.totalBytes)
			}
			if got.SampleCount != len(tt.points) {
				t.Errorf("SampleCount = %d, want %d", got.SampleCount, len(tt.points))
			}
		})
	}
}

func reversed(points []Point) []Point {
	out := make([]Point, len(points))
	for i, p := range points {
		out[len(points)-1-i] = p
	}
	return out
}

func TestProject_SlopeIsExactForPerfectLine(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	points := daily(start, []float64{0, 250, 500, 750, 1000, 1250, 1500})
	got := Project(points, 100_000, start.Add(6*24*time.Hour))
	if got == nil {
		t.Fatal("Project() = nil, want a projection")
	}
	if got.SlopeBytesPerDay < 249.999 || got.SlopeBytesPerDay > 250.001 {
		t.Errorf("SlopeBytesPerDay = %v, want ~250", got.SlopeBytesPerDay)
	}
}

func TestProject_NilSamplesOrEmpty(t *testing.T) {
	if got := Project(nil, 100, time.Now()); got != nil {
		t.Errorf("Project(nil, ...) = %+v, want nil", got)
	}
	if got := Project([]Point{}, 100, time.Now()); got != nil {
		t.Errorf("Project(empty, ...) = %+v, want nil", got)
	}
}
