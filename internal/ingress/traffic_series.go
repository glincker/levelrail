package ingress

import (
	"context"
	"log/slog"
	"time"

	"github.com/glincker/levelrail/internal/trafficclass"
)

// TrafficPoint is one bucket of the request series.
type TrafficPoint struct {
	Start    time.Time `json:"start"`
	Requests int       `json:"requests"`
	Pages    int       `json:"pages"`
	Assets   int       `json:"assets"`
	Errors   int       `json:"errors"`
}

// TrafficSeries buckets requests for an app over [since, until] into n equal steps.
// Hours older than the live ring come from the stored hourly rollups, so a restart keeps history.
func (l *AccessLog) TrafficSeries(ctx context.Context, app string, keys []string, since, until time.Time, n int) []TrafficPoint {
	span := until.Sub(since)
	if l == nil || span <= 0 || n <= 0 {
		return []TrafficPoint{}
	}
	step := span / time.Duration(n)
	if step <= 0 {
		step = time.Second
	}
	out := make([]TrafficPoint, n)
	for i := range out {
		out[i].Start = since.Add(time.Duration(i) * step)
	}
	idx := func(t time.Time) int { return min(max(int(t.Sub(since)/step), 0), n-1) }
	var oldest time.Time
	for _, rec := range l.Since(keys, since) {
		if rec.Time.After(until) {
			continue
		}
		if oldest.IsZero() || rec.Time.Before(oldest) {
			oldest = rec.Time
		}
		p := &out[idx(rec.Time)]
		p.Requests++
		switch rec.Class {
		case trafficclass.Page:
			p.Pages++
		case trafficclass.Asset:
			p.Assets++
		}
		if rec.Status >= 400 {
			p.Errors++
		}
	}
	if !l.hist.on.Load() {
		return out
	}
	cfg := l.hist.cfg.Load()
	if cfg == nil {
		return out
	}
	cut := until
	if !oldest.IsZero() {
		cut = oldest
	}
	cutHour := cut.Truncate(time.Hour)
	rows, err := cfg.st.ListVisitorRollups(ctx, app, "", since.Truncate(time.Hour), cutHour)
	if err != nil {
		cfg.logger.Warn("traffic series history read failed", slog.String("error", err.Error()))
		return out
	}
	for _, r := range rows {
		if !r.HourStart.Before(cutHour) || r.HourStart.Before(since.Truncate(time.Hour)) {
			continue
		}
		p := &out[idx(r.HourStart)]
		p.Requests += r.Requests
		p.Pages += r.Classes[trafficclass.Page]
		p.Assets += r.Classes[trafficclass.Asset]
		p.Errors += r.Status[2] + r.Status[3]
	}
	return out
}
