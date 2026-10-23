package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

func TestPrintIdleFootprint(t *testing.T) {
	var out bytes.Buffer
	printIdleFootprint(&out, footprint.Assessment{})
	if out.Len() != 0 {
		t.Fatalf("empty assessment printed %q", out.String())
	}
	a := footprint.Assessment{
		Status: footprint.StatusOver, Platform: "linux/arm64",
		Current: footprint.Reading{RSSBytes: 300 << 20, CPUPercent: 2, CPUWindowSeconds: 60, Goroutines: 400, OpenFDs: 30},
		Checks: []footprint.MetricCheck{
			{Name: "rss", Unit: "MB", Current: 300, Baseline: 216, Limit: 260, OK: false},
			{Name: "open_fds", Unit: "count", Current: 30, Baseline: 27, Limit: 35, OK: true},
		},
	}
	printIdleFootprint(&out, a)
	got := out.String()
	for _, want := range []string{"idle:", "over_tolerance", "OVER rss", "ok   open_fds", "limit 260.00"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}
