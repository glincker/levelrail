package statuspage

import (
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// UptimeReportRow is one app's availability for a calendar month. It is for
// signed-in operators only: Target is an internal app name.
type UptimeReportRow struct {
	Component string   `json:"component"`
	Target    string   `json:"target"`
	Kind      string   `json:"kind"`
	Uptime    *float64 `json:"uptime_percent"`
	Probes    int      `json:"probes"`
	DownProbe int      `json:"down_probes"`
	DaysSeen  int      `json:"days_with_data"`
	Summary   string   `json:"summary"`
}

// MonthlyReport computes per-component uptime for the month ("2006-01"),
// weighting degraded probes the same way the public bars do.
func MonthlyReport(comps []store.StatusComponent, daily []store.StatusDaily, month string) ([]UptimeReportRow, error) {
	first, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, fmt.Errorf("month must look like 2026-11: %w", err)
	}
	from, to := first.Format("2006-01-02"), first.AddDate(0, 1, -1).Format("2006-01-02")
	t := TuningFromEnv()
	type acc struct{ ok, deg, down, days int }
	by := map[string]*acc{}
	for _, d := range daily {
		if d.Day < from || d.Day > to {
			continue
		}
		a := by[d.ComponentID]
		if a == nil {
			a = &acc{}
			by[d.ComponentID] = a
		}
		a.ok, a.deg, a.down, a.days = a.ok+d.OK, a.deg+d.Degraded, a.down+d.Down, a.days+1
	}
	rows := make([]UptimeReportRow, 0, len(comps))
	for _, c := range comps {
		row := UptimeReportRow{Component: c.DisplayName, Target: c.Target, Kind: c.Kind}
		if a := by[c.ID]; a != nil {
			row.Probes, row.DownProbe, row.DaysSeen = a.ok+a.deg+a.down, a.down, a.days
			if u, has := uptimeFraction(a.ok, a.deg, a.down, t); has {
				pct := roundPct(u)
				row.Uptime = &pct
			}
		}
		if row.Uptime == nil {
			row.Summary = fmt.Sprintf("%s: no data for %s", c.DisplayName, month)
		} else {
			row.Summary = fmt.Sprintf("%s: %.2f%% uptime in %s (%d probes, %d days with data)", c.DisplayName, *row.Uptime, month, row.Probes, row.DaysSeen)
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Component < rows[j].Component })
	return rows, nil
}
