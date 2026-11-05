package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
			until, err := silenceUntil(tt.req, now, 0)
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

func TestSilenceProtectedKindNeedsRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	const plaintext = "write-scoped-silence-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_write_sil", Name: "ci", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	post := func(key string, bearer bool) int {
		body := `{"issue_key":"` + key + `","duration":"30d","severity":"warning","confirm":true}`
		var req *http.Request
		if bearer {
			req = httptest.NewRequest(http.MethodPost, "/api/v1/silences", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+plaintext)
		} else {
			req = authedRequest(t, cookie, http.MethodPost, "/api/v1/silences", body)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	tests := []struct {
		name   string
		key    string
		bearer bool
		want   int
	}{
		{"write token on guard item", "docker_guard:would-deny:high", true, http.StatusForbidden},
		{"write token on guard doctor check", "doctor:Docker API guard", true, http.StatusForbidden},
		{"write token on guard code form", "doctor:docker_guard", true, http.StatusForbidden},
		{"write token on privilege code form", "doctor:docker_privilege", true, http.StatusForbidden},
		{"write token on podman guard name", "doctor:Podman API guard", true, http.StatusForbidden},
		{"write token on podman privilege name", "doctor:Podman privilege", true, http.StatusForbidden},
		{"root on code form", "doctor:docker_privilege", false, http.StatusCreated},
		{"root on podman name", "doctor:Podman privilege", false, http.StatusCreated},
		{"write token on privilege doctor check", "doctor:Docker privilege", true, http.StatusForbidden},
		{"write token on ordinary key", "doctor:disk", true, http.StatusCreated},
		{"root with confirm", "docker_guard:would-deny:high", false, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := post(tt.key, tt.bearer); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProtectedSilenceIsCapped(t *testing.T) {
	now := time.Now()
	protected := createIssueSilenceRequest{IssueKey: "docker_guard:would-deny:high:privileged", Confirm: true}
	tests := []struct {
		name    string
		req     createIssueSilenceRequest
		max     time.Duration
		wantErr bool
	}{
		{"forever over the cap", withDuration(protected, "forever"), 30 * 24 * time.Hour, true},
		{"30d at the cap", withDuration(protected, "30d"), 30 * 24 * time.Hour, false},
		{"30d over a 1w cap", withDuration(protected, "30d"), 7 * 24 * time.Hour, true},
		{"cap 0 allows forever", withDuration(protected, "forever"), 0, false},
		{"unprotected ignores the cap", createIssueSilenceRequest{IssueKey: "doctor:disk", Duration: "forever"}, time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := silenceUntil(tt.req, now, tt.max); (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func withDuration(r createIssueSilenceRequest, d string) createIssueSilenceRequest {
	r.Duration = d
	return r
}
