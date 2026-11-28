package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/apiclient"
)

func TestPrintUpgradeAutoStatus(t *testing.T) {
	tests := []struct {
		name string
		st   *apiclient.UpgradeAutoStatus
		want []string
	}{
		{"absent prints nothing", nil, nil},
		{"blocked says why", &apiclient.UpgradeAutoStatus{State: "blocked", Reason: "systemd is not the init system on this host"}, []string{"status: blocked (systemd is not the init system on this host)"}},
		{"degraded shows the retry", &apiclient.UpgradeAutoStatus{State: "degraded", Reason: "offline", LastCheckAt: "2026-11-20T03:00:00Z", CheckFailures: 3, NextCheckAt: "2026-11-20T03:04:00Z"},
			[]string{"status: degraded (offline)", "3 failed in a row, next try 2026-11-20T03:04:00Z"}},
		{"last result", &apiclient.UpgradeAutoStatus{State: "ready", LastResult: "failed", LastResultReason: "rolled back", LastResultAt: "2026-11-19T03:10:00Z"}, []string{"last result: failed: rolled back (2026-11-19T03:10:00Z)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			printUpgradeAutoStatus(&out, tt.st)
			if len(tt.want) == 0 && out.Len() != 0 {
				t.Fatalf("expected no output, got %q", out.String())
			}
			for _, w := range tt.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("output %q missing %q", out.String(), w)
				}
			}
		})
	}
}
