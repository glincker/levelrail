package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestParseRunAt(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 500, time.UTC)
	cases := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"2026-12-01T03:00:00Z", time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC), false},
		{"2h", time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), false},
		{"30m", time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC), false},
		{"in 2h", time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), false},
		{"  in 30m ", time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC), false},
		{"in ", time.Time{}, true},
		{"-5m", time.Time{}, true},
		{"tomorrow", time.Time{}, true},
	}
	for _, c := range cases {
		got, err := parseRunAt(c.in, now)
		if (err != nil) != c.wantErr {
			t.Fatalf("parseRunAt(%q) error = %v, wantErr %v", c.in, err, c.wantErr)
		}
		if !c.wantErr && !got.Equal(c.want) {
			t.Errorf("parseRunAt(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRun_AppsScheduleSetRunAtRelative(t *testing.T) {
	var gotBody apiclient.SetAppScheduleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppScheduleResource{ServiceName: "web", Branch: gotBody.Branch, Timezone: "UTC", Enabled: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "schedule", "set", "web", "--run-at", "2h", "--branch", "main", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	at, err := time.Parse(time.RFC3339, gotBody.RunAt)
	if err != nil {
		t.Fatalf("request run_at = %q, want RFC 3339: %v", gotBody.RunAt, err)
	}
	if d := time.Until(at); d < 119*time.Minute || d > 121*time.Minute {
		t.Errorf("run_at is %v from now, want about 2h", d)
	}
	if gotBody.Cron != "" {
		t.Errorf("cron = %q, want empty", gotBody.Cron)
	}
	if !strings.Contains(stdout.String(), gotBody.RunAt) {
		t.Errorf("stdout = %q, want the absolute timestamp %s printed back", stdout.String(), gotBody.RunAt)
	}
}

func TestRun_AppsScheduleSetCronAndRunAtExclusive(t *testing.T) {
	for _, args := range [][]string{
		{"apps", "schedule", "set", "web", "--cron", "0 3 * * *", "--run-at", "2h", "--branch", "main"},
		{"apps", "schedule", "set", "web", "--branch", "main"},
		{"apps", "schedule", "set", "web", "--run-at", "soon", "--branch", "main"},
	} {
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got == exitOK {
			t.Errorf("run(%v) exit = OK, want a validation failure", args)
		}
	}
}
