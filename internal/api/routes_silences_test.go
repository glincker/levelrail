package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSilenceUntil(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		req       createIssueSilenceRequest
		wantErr   bool
		wantUntil time.Duration
		forever   bool
	}{
		{"warning 1h", createIssueSilenceRequest{IssueKey: "doctor:disk", Duration: "1h"}, false, time.Hour, false},
		{"warning forever", createIssueSilenceRequest{IssueKey: "doctor:disk", Duration: "forever", Severity: "warning"}, false, 0, true},
		{"critical 30d needs no confirm", createIssueSilenceRequest{IssueKey: "node:a", Duration: "30d", Severity: "critical"}, false, 30 * 24 * time.Hour, false},
		{"critical forever unconfirmed", createIssueSilenceRequest{IssueKey: "node:a", Duration: "forever", Severity: "critical"}, true, 0, false},
		{"critical forever confirmed", createIssueSilenceRequest{IssueKey: "node:a", Duration: "forever", Severity: "critical", Confirm: true}, false, 0, true},
		{"protected 1h unconfirmed", createIssueSilenceRequest{IssueKey: "login_anomaly:x", Duration: "1h"}, true, 0, false},
		{"protected 1h confirmed", createIssueSilenceRequest{IssueKey: "login_anomaly:x", Duration: "1h", Confirm: true}, false, time.Hour, false},
		{"bad duration", createIssueSilenceRequest{IssueKey: "doctor:disk", Duration: "90d"}, true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			until, err := silenceUntil(tt.req, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if tt.forever {
				if until != nil {
					t.Fatalf("until = %v, want forever", until)
				}
				return
			}
			if until == nil || !until.Equal(now.Add(tt.wantUntil)) {
				t.Fatalf("until = %v, want now+%v", until, tt.wantUntil)
			}
		})
	}
}

func TestSilencedDoctorCount(t *testing.T) {
	now := time.Now()
	checks := []doctorCheckResource{
		{Name: "disk", Status: "warn"},
		{Name: "tls", Status: "ok"},
		{Name: "dns", Status: "fail"},
	}
	silences := []store.Silence{{IssueKey: "doctor:disk"}, {IssueKey: "doctor:tls"}}
	if got := silencedDoctorCount(checks, silences, now); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
}
