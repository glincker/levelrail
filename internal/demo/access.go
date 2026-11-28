package demo

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/glincker/levelrail/internal/ingress"
)

// Access answers the edge traffic questions with deterministic fictional
// numbers, so Pages traffic and app visitor panels have data without a proxy.
type Access struct{ Now func() time.Time }

var accessPaths = []struct {
	path   string
	status int
	weight int
}{
	{"/", 200, 30}, {"/pricing", 200, 14}, {"/blog/launch-notes", 200, 9}, {"/docs/getting-started", 200, 8},
	{"/assets/app.css", 200, 12}, {"/assets/app.js", 200, 12}, {"/old-pricing", 404, 3}, {"/blog/feed.xml.bak", 404, 2},
	{"/wp-login.php", 404, 2}, {"/changelog", 200, 5}, {"/contact", 200, 3},
}

func (a Access) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func appFactor(app string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(app))
	return 60 + int(h.Sum32()%90)
}

// AppAccess returns recent requests, newest first.
func (a Access) AppAccess(app string, f ingress.AccessFilter) []ingress.AccessRecord {
	now := a.now()
	n := 320 + appFactor(app)%100
	if f.Limit > 0 && n > f.Limit {
		n = f.Limit
	}
	span := now.Sub(f.Since)
	if f.Since.IsZero() || span > 24*time.Hour {
		span = 24 * time.Hour
	}
	step := span / time.Duration(n+1)
	total := 0
	for _, p := range accessPaths {
		total += p.weight
	}
	out := make([]ingress.AccessRecord, 0, n)
	for i := 0; i < n; i++ {
		pick, acc := (i*7+appFactor(app))%total, 0
		for _, p := range accessPaths {
			acc += p.weight
			if pick < acc {
				out = append(out, ingress.AccessRecord{
					Time: now.Add(-time.Duration(i) * step), Host: app + ".demo.example.com", Method: "GET",
					Path: p.path, Status: p.status, Millis: float64(4 + (i*13)%40), BytesOut: int64(2_000 + (i*911)%48_000),
				})
				break
			}
		}
	}
	return out
}

// AppVisitors summarizes the window with numbers scaled by its length.
func (a Access) AppVisitors(_ context.Context, app string, o ingress.VisitorSummaryOptions) ingress.VisitorSummary {
	hours := int(o.Until.Sub(o.Since) / time.Hour)
	if hours < 1 {
		hours = 24
	}
	reqs := hours * appFactor(app) / 5
	if hours <= 24 {
		reqs = 400 + appFactor(app)
	}
	sum := ingress.VisitorSummary{
		Requests: reqs, Visitors: reqs / 4, Status: map[string]int{"2xx": reqs * 93 / 100, "3xx": reqs * 4 / 100, "4xx": reqs * 3 / 100, "5xx": reqs / 100},
		Classes:    ingress.ClassCounts{Page: reqs * 55 / 100, Asset: reqs * 35 / 100, Bot: reqs * 7 / 100, Other: reqs * 3 / 100},
		TopRefs:    []ingress.CountedItem{{Label: "news.example.com", Count: reqs / 9}, {Label: "social.example.com", Count: reqs / 14}, {Label: "(direct)", Count: reqs / 3}},
		SaltPeriod: "daily",
	}
	for _, p := range accessPaths {
		sum.TopPaths = append(sum.TopPaths, ingress.CountedItem{Label: p.path, Count: reqs * p.weight / 100})
	}
	return sum
}

// AppTrafficSeries buckets a smooth daily curve across the window.
func (a Access) AppTrafficSeries(_ context.Context, app string, since, until time.Time, buckets int) []ingress.TrafficPoint {
	if buckets < 1 {
		return nil
	}
	step := until.Sub(since) / time.Duration(buckets)
	base := appFactor(app) / 6
	out := make([]ingress.TrafficPoint, buckets)
	for i := range out {
		start := since.Add(time.Duration(i) * step)
		wave := 6 + (start.Hour()*7+i*3)%14
		req := base * wave / 8
		out[i] = ingress.TrafficPoint{Start: start, Requests: req, Pages: req * 55 / 100, Assets: req * 40 / 100, Errors: req / 40}
	}
	return out
}
