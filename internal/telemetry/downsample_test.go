package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestAutoStep(t *testing.T) {
	cases := []struct {
		name  string
		span  time.Duration
		max   int
		floor time.Duration
		want  time.Duration
	}{
		{"short range keeps floor", time.Hour, 600, 15 * time.Second, 15 * time.Second},
		{"6h", 6 * time.Hour, 600, 15 * time.Second, time.Minute},
		{"24h", 24 * time.Hour, 600, 15 * time.Second, 5 * time.Minute},
		{"7d", 7 * 24 * time.Hour, 600, 15 * time.Second, 30 * time.Minute},
		{"30d", 30 * 24 * time.Hour, 600, 15 * time.Second, 2 * time.Hour},
		{"floor wins", time.Hour, 600, 5 * time.Minute, 5 * time.Minute},
		{"no cap", 7 * 24 * time.Hour, 0, 15 * time.Second, 15 * time.Second},
		{"zero span", 0, 600, 15 * time.Second, 15 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AutoStep(tc.span, tc.max, tc.floor); got != tc.want {
				t.Errorf("AutoStep = %s, want %s", got, tc.want)
			}
		})
	}
}

// Seven days of 15s samples is 40320 raw points; the downsampled series must
// stay within the cap and keep the spike visible through Max.
func TestAggregateSevenDaysStaysBounded(t *testing.T) {
	const maxPoints = 600
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	span := 7 * 24 * time.Hour
	var samples []Sample
	for ts := from; ts.Before(from.Add(span)); ts = ts.Add(15 * time.Second) {
		v := 10.0
		if ts.Equal(from.Add(3*24*time.Hour + 7*time.Minute)) {
			v = 950
		}
		samples = append(samples, Sample{Timestamp: ts, Value: v})
	}
	if len(samples) != 40320 {
		t.Fatalf("raw samples = %d", len(samples))
	}
	step := AutoStep(span, maxPoints, MinRequestStep)
	start := time.Now()
	pts := Aggregate(samples, from, step)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("aggregation took %s, want under 2s", elapsed)
	}
	if len(pts) > maxPoints {
		t.Fatalf("points = %d, want <= %d", len(pts), maxPoints)
	}
	var peak float64
	for _, p := range pts {
		peak = max(peak, p.Max)
	}
	if peak != 950 {
		t.Errorf("peak = %v, want the spike to survive downsampling", peak)
	}
}

func TestAggregateMaxAndCount(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	samples := []Sample{
		{Timestamp: from.Add(1 * time.Second), Value: 1},
		{Timestamp: from.Add(2 * time.Second), Value: 9},
		{Timestamp: from.Add(70 * time.Second), Value: 4},
	}
	got := Aggregate(samples, from, time.Minute)
	if len(got) != 2 || got[0].Max != 9 || got[0].Value != 5 || got[0].Count != 2 || got[1].Max != 4 {
		t.Errorf("aggregate = %+v", got)
	}
}

func TestShiftPoints(t *testing.T) {
	at := time.Unix(1000, 0).UTC()
	got := ShiftPoints([]AggregatedPoint{{Timestamp: at, Value: 1}}, time.Hour)
	if !got[0].Timestamp.Equal(at.Add(time.Hour)) {
		t.Errorf("shifted = %v", got[0].Timestamp)
	}
}

func TestNormalizeRoute(t *testing.T) {
	cases := []struct {
		path  string
		depth int
		want  string
	}{
		{"/", 3, "/"},
		{"", 3, "/"},
		{"/api/orders", 3, "/api/orders"},
		{"/api/orders/12345", 3, "/api/orders/:id"},
		{"/api/users/3f2a9c1e-77aa-4c3e-9b1e-0123456789ab/profile", 3, "/api/users/:id/*"},
		{"/reset/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", 3, "/reset/:id"},
		{"/a/b/c/d/e", 3, "/a/b/c/*"},
		{"/v2/items/abcdef12", 2, "/v2/items/*"},
		{"/static/app.js", 3, "/static/app.js"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := NormalizeRoute(tc.path, tc.depth); got != tc.want {
				t.Errorf("NormalizeRoute(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestObserveRouteCapsCardinality(t *testing.T) {
	var w RequestWindow
	for i := 0; i < 10; i++ {
		w.ObserveRoute("/r"+string(rune('a'+i)), 200, 1, 3)
	}
	if len(w.Routes) != 4 {
		t.Fatalf("routes = %d, want 3 plus overflow", len(w.Routes))
	}
	if w.Routes[OverflowRoute].Requests != 7 {
		t.Errorf("overflow = %+v", w.Routes[OverflowRoute])
	}
}

func TestBreakdownRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	var w RequestWindow
	w.Requests, w.Status2xx, w.Status5xx = 3, 2, 1
	w.ObserveRoute("/a", 200, 10, 10)
	w.ObserveRoute("/a", 200, 30, 10)
	w.ObserveRoute("/b", 503, 100, 10)
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w}, at); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w}, at.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	b, err := db.QueryBreakdown(ctx, "service:web", at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	top := b.TopRoutes(5)
	if len(top) != 2 || top[0].Route != "/a" || top[0].Requests != 4 || top[0].LatencyMSSum != 80 {
		t.Errorf("top = %+v", top)
	}
	st := b.SortedStatuses()
	if len(st) != 2 || st[0].Status != 200 || st[0].Count != 4 || st[1].Status != 503 {
		t.Errorf("statuses = %+v", st)
	}
}

func TestParseFieldFilter(t *testing.T) {
	cases := []struct {
		in      string
		want    FieldFilter
		wantErr bool
	}{
		{"status=500", FieldFilter{"status", "=", "500"}, false},
		{"status!=200", FieldFilter{"status", "!=", "200"}, false},
		{"msg~timeout", FieldFilter{"msg", "~", "timeout"}, false},
		{"latency>=250", FieldFilter{"latency", ">=", "250"}, false},
		{"req.user.id=7", FieldFilter{"req.user.id", "=", "7"}, false},
		{"url=/x?a=b", FieldFilter{"url", "=", "/x?a=b"}, false},
		{"latency>abc", FieldFilter{}, true},
		{"=x", FieldFilter{}, true},
		{"nokey", FieldFilter{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseFieldFilter(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMatchFieldFilters(t *testing.T) {
	entry := LogEntry{Structured: true, FieldsJSON: `{"level":"error","status":503,"req":{"path":"/pay","ms":1200.5},"msg":"Upstream Timeout"}`}
	plain := LogEntry{Message: "hello"}
	cases := []struct {
		name   string
		e      LogEntry
		filter string
		want   bool
	}{
		{"eq string", entry, "level=error", true},
		{"eq number", entry, "status=503", true},
		{"neq", entry, "status!=200", true},
		{"nested", entry, "req.path=/pay", true},
		{"contains case-insensitive", entry, "msg~timeout", true},
		{"gt", entry, "req.ms>1000", true},
		{"lt fails", entry, "req.ms<1000", false},
		{"missing key eq", entry, "user=1", false},
		{"missing key neq", entry, "user!=1", true},
		{"plain line eq", plain, "level=error", false},
		{"plain line neq", plain, "level!=error", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := ParseFieldFilter(tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if got := MatchFieldFilters(tc.e, []FieldFilter{f}); got != tc.want {
				t.Errorf("match = %v, want %v", got, tc.want)
			}
		})
	}
}
