package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestProbeVerdict(t *testing.T) {
	http := catalog.ManifestHealthcheck{Path: "/"}
	login := catalog.ManifestHealthcheck{Path: "/", Expect: 401}
	tcp := catalog.ManifestHealthcheck{Kind: "tcp"}
	tests := []struct {
		name      string
		h         catalog.ManifestHealthcheck
		status    int
		connected bool
		want      bool
	}{
		{"200", http, 200, false, true},
		{"redirect", http, 302, false, true},
		{"399", http, 399, false, true},
		{"400", http, 400, false, false},
		{"401 undeclared", http, 401, false, false},
		{"401 declared", login, 401, false, true},
		{"403 when 401 declared", login, 403, false, false},
		{"500", http, 500, false, false},
		{"no answer", http, 0, false, false},
		{"tcp up", tcp, 0, true, true},
		{"tcp down", tcp, 0, false, false},
	}
	for _, tt := range tests {
		if got, _ := probeVerdict(tt.h, tt.status, tt.connected); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestOverrideKeepsProbePort(t *testing.T) {
	got := overrideWithProbe(nil, []string{"web", "db"}, "", "web", 8080)
	if !strings.Contains(got, `ports: !override ["127.0.0.1::8080"]`) {
		t.Errorf("probe port missing:\n%s", got)
	}
	if strings.Count(got, "ports: !reset []") != 1 {
		t.Errorf("other services must lose host ports:\n%s", got)
	}
}
