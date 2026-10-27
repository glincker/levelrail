package main

import (
	"strings"
	"testing"
)

func TestUpWaitsForHealthyDeploy(t *testing.T) {
	tests := []struct {
		name        string
		outcome     string
		extra       []string
		wantCode    int
		wantErr     string
		wantPolled  bool
		wantURLOnOK bool
	}{
		{"healthy", "", nil, exitOK, "deploy b1: healthy", true, true},
		{"failed shows reason", "failed", nil, exitDeployNotHealthy, "reason: build exploded", true, false},
		{"still running at timeout", "in_progress", []string{"--timeout", "1ms"}, exitDeployTimeout, "deploy b1: timeout", true, false},
		{"no-wait returns at once", "failed", []string{"--no-wait"}, exitOK, "follow it with", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, srv := newFakeUp(t)
			f.outcome = tc.outcome
			dir := newUpProject(t)
			code, out, errOut := runUpCLI(t, srv, dir, append([]string{"--allow-unpushed"}, tc.extra...)...)
			if code != tc.wantCode {
				t.Fatalf("code = %d, want %d\nstderr: %s", code, tc.wantCode, errOut)
			}
			if !strings.Contains(errOut, tc.wantErr) {
				t.Fatalf("stderr lacks %q:\n%s", tc.wantErr, errOut)
			}
			if polled := f.calls["GET /api/v1/apps/shop/deploys/b1"] > 0; polled != tc.wantPolled {
				t.Fatalf("polled deploy = %v, want %v", polled, tc.wantPolled)
			}
			if has := strings.Contains(out, "https://shop"); has != tc.wantURLOnOK {
				t.Fatalf("stdout URL = %v, want %v (%q)", has, tc.wantURLOnOK, out)
			}
		})
	}
}
