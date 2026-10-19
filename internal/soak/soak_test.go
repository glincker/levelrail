package soak

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func series(n int, f func(i int) Sample) []Sample {
	out := make([]Sample, n)
	base := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	for i := range out {
		out[i] = f(i)
		out[i].At = base.Add(time.Duration(i) * time.Minute)
	}
	return out
}

func TestEvaluate(t *testing.T) {
	lim := Limits{RSSGrowthPct: 20, GoroutineGrowth: 50, FDGrowth: 20, WALMaxBytes: 64 << 20, ErrorRatePct: 1}
	flat := func(i int) Sample {
		return Sample{RSSBytes: 200 << 20, Goroutines: 270, OpenFDs: 60, WALBytes: 1 << 20, Requests: int64(i * 100), Errors: 0}
	}
	tests := []struct {
		name    string
		samples []Sample
		want    bool
		contain string
	}{
		{"flat run passes", series(40, flat), true, ""},
		{"too few samples", series(2, flat), false, "samples"},
		{"rss leak", series(40, func(i int) Sample {
			s := flat(i)
			s.RSSBytes += uint64(i) * (8 << 20) //nolint:gosec // i is a small non-negative loop index
			return s
		}), false, "RSS"},
		{"goroutine leak", series(40, func(i int) Sample { s := flat(i); s.Goroutines += i * 5; return s }), false, "goroutines"},
		{"fd leak", series(40, func(i int) Sample { s := flat(i); s.OpenFDs += i; return s }), false, "file descriptors"},
		{"wal never checkpointed", series(40, func(i int) Sample { s := flat(i); s.WALBytes = int64(i) * (10 << 20); return s }), false, "WAL"},
		{"errors", series(40, func(i int) Sample { s := flat(i); s.Errors = s.Requests / 10; return s }), false, "error rate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Evaluate(tt.samples, lim)
			if res.Pass != tt.want {
				t.Fatalf("pass = %v, want %v: %v", res.Pass, tt.want, res.Violations)
			}
			if tt.contain != "" && !strings.Contains(strings.Join(res.Violations, ";"), tt.contain) {
				t.Fatalf("violations %v do not mention %q", res.Violations, tt.contain)
			}
		})
	}
}

func TestRatchetRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ratchet.json")
	if _, ok, err := LoadRatchet(path); err != nil || ok {
		t.Fatalf("missing file: ok=%v err=%v", ok, err)
	}
	if err := SaveRatchet(path, Summary{EndRSSBytes: 100, EndGoroutines: 10, EndOpenFDs: 5}); err != nil {
		t.Fatal(err)
	}
	prev, ok, err := LoadRatchet(path)
	if err != nil || !ok || prev.EndRSSBytes != 100 {
		t.Fatalf("load: %+v ok=%v err=%v", prev, ok, err)
	}
	res := Result{Pass: true, Summary: Summary{EndRSSBytes: 130, EndGoroutines: 10, EndOpenFDs: 5}}
	CheckRatchet(&res, prev, 10)
	if res.Pass || len(res.Violations) != 1 {
		t.Fatalf("expected one RSS violation, got %+v", res)
	}
	ok2 := Result{Pass: true, Summary: Summary{EndRSSBytes: 105, EndGoroutines: 10, EndOpenFDs: 5}}
	CheckRatchet(&ok2, prev, 10)
	if !ok2.Pass {
		t.Fatalf("within slack should pass: %+v", ok2)
	}
}

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, series(2, func(int) Sample { return Sample{RSSBytes: 5, Goroutines: 3, OpenFDs: 2} })); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "timestamp,rss_bytes") {
		t.Fatalf("unexpected csv:\n%s", buf.String())
	}
}
