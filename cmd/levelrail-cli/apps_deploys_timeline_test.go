package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestTimelineBar(t *testing.T) {
	since := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	fin := since.Add(12 * time.Hour)
	tests := []struct {
		name string
		d    apiclient.TimelineDeploy
		want byte
		from int
	}{
		{"ready run starts mid window", apiclient.TimelineDeploy{Status: "ready", StartedAt: since.Add(6 * time.Hour), FinishedAt: &fin}, '=', 10},
		{"in flight runs to the end", apiclient.TimelineDeploy{Status: "building", StartedAt: since.Add(23 * time.Hour)}, '~', 38},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timelineBar(tt.d, since, until)
			if len(got) != timelineBarWidth || got[tt.from] != tt.want {
				t.Fatalf("bar %q does not hold %q at %d", got, tt.want, tt.from)
			}
		})
	}
}

func TestPrintTimelineEmpty(t *testing.T) {
	var buf bytes.Buffer
	printTimeline(&buf, apiclient.TimelinePage{})
	if !strings.Contains(buf.String(), "no deploys") {
		t.Fatalf("got %q", buf.String())
	}
}
