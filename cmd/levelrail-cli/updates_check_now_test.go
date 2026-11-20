package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintNextWindows(t *testing.T) {
	tests := []struct {
		name string
		ws   []apiclient.UpgradeWindow
		tz   string
		want []string
	}{
		{name: "server zone and utc", tz: "Europe/Berlin", ws: []apiclient.UpgradeWindow{{Start: "2026-11-21T02:00:00Z"}, {Start: "2026-11-22T02:00:00Z"}},
			want: []string{"Sat 2026-11-21 03:00 (Europe/Berlin) / 02:00 UTC", "Sun 2026-11-22 03:00"}},
		{name: "unknown zone falls back to utc", tz: "Nowhere/Land", ws: []apiclient.UpgradeWindow{{Start: "2026-11-21T03:00:00Z"}},
			want: []string{"Sat 2026-11-21 03:00 (UTC) / 03:00 UTC"}},
		{name: "none prints nothing", tz: "UTC"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			printNextWindows(&b, tc.ws, tc.tz)
			for _, w := range tc.want {
				if !strings.Contains(b.String(), w) {
					t.Fatalf("output %q lacks %q", b.String(), w)
				}
			}
			if len(tc.want) == 0 && b.Len() != 0 {
				t.Fatalf("unexpected output %q", b.String())
			}
		})
	}
}
