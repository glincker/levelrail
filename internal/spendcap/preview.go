package spendcap

import (
	"sort"
	"time"
)

// AppLoad is what a pause would stop for one app. Reclaimable cost is the
// part a pause removes (compute and egress); storage and idle node cost stay.
type AppLoad struct {
	Name        string
	Replicas    int
	MonthToDate float64
	RecentDaily []float64
}

// PreviewApp is one app's line in a pause preview.
type PreviewApp struct {
	Name            string  `json:"name"`
	Replicas        int     `json:"replicas"`
	DailyRunRate    float64 `json:"daily_run_rate"`
	EstimatedSaving float64 `json:"estimated_saving"`
}

// Preview is what pausing a scope would do. It is computed, never applied.
type Preview struct {
	Apps            []PreviewApp `json:"apps"`
	TotalReplicas   int          `json:"total_replicas"`
	RemainingDays   float64      `json:"remaining_days"`
	EstimatedSaving float64      `json:"estimated_saving"`
	DryRun          bool         `json:"dry_run"`
}

// PreviewPause estimates stopping every replica of each app until month end.
// The run rate is the recent daily average, or month to date over elapsed
// days when no full day has been recorded yet.
func PreviewPause(apps []AppLoad, now time.Time) Preview {
	u := now.UTC()
	start := time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	elapsed := u.Sub(start).Hours() / 24
	remaining := NextPeriodStart(u).Sub(start).Hours()/24 - elapsed
	out := Preview{Apps: []PreviewApp{}, RemainingDays: remaining, DryRun: true}
	for _, a := range apps {
		rate := 0.0
		switch {
		case len(a.RecentDaily) > 0:
			for _, d := range a.RecentDaily {
				rate += d
			}
			rate /= float64(len(a.RecentDaily))
		case elapsed > 0:
			rate = a.MonthToDate / elapsed
		}
		line := PreviewApp{Name: a.Name, Replicas: a.Replicas, DailyRunRate: rate, EstimatedSaving: rate * remaining}
		out.Apps = append(out.Apps, line)
		out.TotalReplicas += a.Replicas
		out.EstimatedSaving += line.EstimatedSaving
	}
	sort.Slice(out.Apps, func(i, j int) bool { return out.Apps[i].Name < out.Apps[j].Name })
	return out
}
