package forecast_test

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/kit/forecast"
)

func ExampleProject() {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const gib = 1 << 30
	var points []forecast.Point
	for day := 0; day < 7; day++ {
		points = append(points, forecast.Point{
			Timestamp: start.AddDate(0, 0, day),
			UsedBytes: float64(10+day*5) * gib,
		})
	}
	now := start.AddDate(0, 0, 6)
	p := forecast.Project(points, 100*gib, now)
	fmt.Printf("growing %.0f GiB/day, full in %.0f days\n", p.SlopeBytesPerDay/gib, p.DaysUntilFull)
	// Output:
	// growing 5 GiB/day, full in 12 days
}
