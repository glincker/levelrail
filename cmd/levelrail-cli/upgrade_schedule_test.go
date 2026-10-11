package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type scheduleFake struct {
	schedule *apiclient.UpgradeScheduleRequest
	cancels  int
	policy   apiclient.UpgradePolicy
	put      *apiclient.UpgradePolicyRequest
	backups  int
}

func scheduleServer(t *testing.T, f *scheduleFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/updates/schedule":
			var req apiclient.UpgradeScheduleRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.schedule = &req
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(apiclient.UpgradeSchedule{ID: "us_1", TargetVersion: "v1.1.0", State: "pending", RunAt: "2026-10-12T01:00:00Z", RetryUntil: "2026-10-12T03:00:00Z"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/updates/schedule":
			f.cancels++
			_ = json.NewEncoder(w).Encode(apiclient.UpgradeSchedule{ID: "us_1", TargetVersion: "v1.1.0", State: "cancelled"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/updates/schedule":
			_ = json.NewEncoder(w).Encode(apiclient.UpgradeScheduleStatus{
				Available: true, ServerTimezone: "Europe/Berlin",
				Active: &apiclient.UpgradeSchedule{ID: "us_1", TargetVersion: "v1.1.0", State: "blocked", RunAt: "2026-10-12T01:00:00Z", RetryUntil: "2026-10-12T03:00:00Z", LastReason: "Free disk space: 10 MiB free"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/updates/auto":
			_ = json.NewEncoder(w).Encode(f.policy)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/updates/auto":
			var req apiclient.UpgradePolicyRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.put = &req
			_ = json.NewEncoder(w).Encode(apiclient.UpgradePolicy{Mode: req.Mode, Effective: req.Mode, WindowCron: req.WindowCron, WindowDurationMinutes: req.WindowDurationMinutes, Timezone: req.Timezone, Paused: req.Paused})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/updates/backup-now":
			f.backups++
			w.WriteHeader(http.StatusCreated)
			var res apiclient.UpgradeBackupNowResult
			res.Backup.Name = "levelrail-1.db"
			_ = json.NewEncoder(w).Encode(res)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_UpgradeSchedule(t *testing.T) {
	f := &scheduleFake{}
	srv := scheduleServer(t, f)

	t.Run("schedule at an absolute time", func(t *testing.T) {
		out, _ := runCLIExpectOK(t, []string{"upgrade", "schedule", "--at", "2026-10-12T03:00:00+02:00", "--version", "v1.1.0", "--ack-breaking", "a,b", "--api-url", srv.URL})
		if f.schedule == nil || f.schedule.At != "2026-10-12T01:00:00Z" || f.schedule.Target != "v1.1.0" || strings.Join(f.schedule.Ack, ",") != "a,b" {
			t.Fatalf("request = %+v", f.schedule)
		}
		if !strings.Contains(out, "scheduled the upgrade to v1.1.0") {
			t.Errorf("stdout = %s", out)
		}
	})
	t.Run("a local time goes to the server with its timezone", func(t *testing.T) {
		runCLIExpectOK(t, []string{"upgrade", "schedule", "--at", "2026-10-12 03:00", "--timezone", "Europe/Berlin", "--api-url", srv.URL})
		if f.schedule.AtLocal != "2026-10-12T03:00" || f.schedule.Timezone != "Europe/Berlin" || f.schedule.At != "" {
			t.Fatalf("request = %+v", f.schedule)
		}
	})
	t.Run("next maintenance window", func(t *testing.T) {
		runCLIExpectOK(t, []string{"upgrade", "schedule", "--window", "--replace", "--api-url", srv.URL})
		if !f.schedule.Window || !f.schedule.Replace {
			t.Fatalf("request = %+v", f.schedule)
		}
	})
	t.Run("at and window together are refused", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := run("levelrail-cli-test", []string{"upgrade", "schedule", "--at", "2026-10-12 03:00", "--window", "--api-url", srv.URL}, &out, &errb, envMap()); code == exitOK {
			t.Fatalf("exit = %d", code)
		}
	})
	t.Run("an unparseable time is refused", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := run("levelrail-cli-test", []string{"upgrade", "schedule", "--at", "tomorrow-ish", "--api-url", srv.URL}, &out, &errb, envMap()); code == exitOK {
			t.Fatalf("exit = %d", code)
		}
	})
	t.Run("show prints the server timezone and the blocker", func(t *testing.T) {
		out, _ := runCLIExpectOK(t, []string{"upgrade", "schedule", "show", "--api-url", srv.URL})
		for _, want := range []string{"Europe/Berlin", "blocked", "Free disk space"} {
			if !strings.Contains(out, want) {
				t.Errorf("stdout missing %q: %s", want, out)
			}
		}
	})
	t.Run("cancel", func(t *testing.T) {
		runCLIExpectOK(t, []string{"upgrade", "schedule", "cancel", "--api-url", srv.URL})
		if f.cancels != 1 {
			t.Fatalf("cancels = %d", f.cancels)
		}
	})
}

func TestRun_UpgradeAutoAndBackupNow(t *testing.T) {
	f := &scheduleFake{policy: apiclient.UpgradePolicy{
		Mode: "patch", Effective: "patch", WindowCron: "0 3 * * *", WindowDurationMinutes: 120, Timezone: "UTC", MinReleaseAgeDays: 3,
		Decision: "held: 1 breaking change", DecisionTag: "v1.2.0",
	}}
	srv := scheduleServer(t, f)

	out, _ := runCLIExpectOK(t, []string{"upgrade", "auto", "get", "--api-url", srv.URL})
	for _, want := range []string{"automatic upgrades: patch", "0 3 * * *", "held: 1 breaking change"} {
		if !strings.Contains(out, want) {
			t.Errorf("get stdout missing %q: %s", want, out)
		}
	}

	runCLIExpectOK(t, []string{"upgrade", "auto", "set", "--mode", "any", "--pause", "--api-url", srv.URL})
	if f.put == nil || f.put.Mode != "any" || !f.put.Paused || f.put.WindowCron != "0 3 * * *" || f.put.MinReleaseAgeDays != 3 {
		t.Fatalf("set must change only the given flags: %+v", f.put)
	}

	runCLIExpectOK(t, []string{"upgrade", "auto", "set", "--resume", "--min-age-days", "0", "--window-duration", "90m", "--api-url", srv.URL})
	if f.put.Paused || f.put.MinReleaseAgeDays != 0 || f.put.WindowDurationMinutes != 90 || f.put.Mode != "patch" {
		t.Fatalf("second set = %+v", f.put)
	}

	out, _ = runCLIExpectOK(t, []string{"upgrade", "backup-now", "--api-url", srv.URL})
	if f.backups != 1 || !strings.Contains(out, "levelrail-1.db") {
		t.Fatalf("backups=%d stdout=%s", f.backups, out)
	}
}
