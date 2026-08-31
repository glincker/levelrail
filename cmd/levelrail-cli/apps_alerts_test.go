package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsAlertsCreate_Threshold(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(alertRuleResource{
			ID: "alr_1", Name: gotBody.Name, Kind: gotBody.Kind, Metric: gotBody.Metric,
			Comparator: gotBody.Comparator, Threshold: gotBody.Threshold, Enabled: gotBody.Enabled,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "alerts", "create", "web",
		"--name", "high-cpu", "--kind", "threshold", "--metric", "cpu_percent", "--comparator", ">", "--threshold", "80",
		"--api-url", srv.URL, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/alerts" {
		t.Errorf("path = %q, want /api/v1/apps/web/alerts", gotPath)
	}
	if gotBody.Kind != "threshold" || gotBody.Metric != "cpu_percent" || gotBody.Comparator != ">" || gotBody.Threshold != 80 {
		t.Errorf("request body = %+v, want a threshold rule on cpu_percent > 80", gotBody)
	}
	if !strings.Contains(stdout.String(), `"id": "alr_1"`) {
		t.Errorf("stdout = %q, want the created rule as JSON", stdout.String())
	}
}

func TestRun_AppsAlertsCreate_CertExpiry_NoExtraFieldsRequired(t *testing.T) {
	var gotBody createAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(alertRuleResource{ID: "alr_2", Name: gotBody.Name, Kind: gotBody.Kind, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "alerts", "create", "web",
		"--name", "cert-expiry-watch", "--kind", "cert_expiry",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Kind != "cert_expiry" {
		t.Errorf("request body Kind = %q, want cert_expiry", gotBody.Kind)
	}
	if !strings.Contains(stdout.String(), `alert rule "cert-expiry-watch" (id alr_2, kind cert_expiry) created for app "web"`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout.String())
	}
}

func TestRun_AppsAlertsCreate_PatchStatus_NoExtraFieldsRequired(t *testing.T) {
	var gotBody createAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(alertRuleResource{ID: "alr_3", Name: gotBody.Name, Kind: gotBody.Kind, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "alerts", "create", "web",
		"--name", "patch-status-watch", "--kind", "patch_status",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Kind != "patch_status" {
		t.Errorf("request body Kind = %q, want patch_status", gotBody.Kind)
	}
	if !strings.Contains(stdout.String(), `alert rule "patch-status-watch" (id alr_3, kind patch_status) created for app "web"`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout.String())
	}
}

func TestRun_AppsAlertsCreate_NodeDiskSpace_NoExtraFieldsRequired(t *testing.T) {
	var gotBody createAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(alertRuleResource{ID: "alr_4", Name: gotBody.Name, Kind: gotBody.Kind, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "alerts", "create", "web",
		"--name", "disk-space-watch", "--kind", "node_disk_space",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Kind != "node_disk_space" {
		t.Errorf("request body Kind = %q, want node_disk_space", gotBody.Kind)
	}
	if !strings.Contains(stdout.String(), `alert rule "disk-space-watch" (id alr_4, kind node_disk_space) created for app "web"`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout.String())
	}
}

func TestRun_AppsAlertsCreate_ScheduledTaskFailure(t *testing.T) {
	var gotBody createAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(alertRuleResource{
			ID: "alr_3", Name: gotBody.Name, Kind: gotBody.Kind,
			ScheduledTaskID: gotBody.ScheduledTaskID, RestartCountThreshold: gotBody.RestartCountThreshold, Enabled: gotBody.Enabled,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "alerts", "create", "web",
		"--name", "cleanup-failing", "--kind", "scheduled_task_failure", "--scheduled-task-id", "sct_1", "--restart-count-threshold", "3",
		"--api-url", srv.URL, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Kind != "scheduled_task_failure" || gotBody.ScheduledTaskID != "sct_1" || gotBody.RestartCountThreshold != 3 {
		t.Errorf("request body = %+v, want a scheduled_task_failure rule on sct_1 with threshold 3", gotBody)
	}
	if !strings.Contains(stdout.String(), `"id": "alr_3"`) {
		t.Errorf("stdout = %q, want the created rule as JSON", stdout.String())
	}
}

func TestRun_AppsAlertsCreate_ScheduledTaskFailureMissingTaskID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "alerts", "create", "web", "--name", "x", "--kind", "scheduled_task_failure", "--restart-count-threshold", "3",
	}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--scheduled-task-id is required") {
		t.Errorf("stderr = %q, want a missing --scheduled-task-id error", stderr.String())
	}
}

func TestRun_AppsAlertsCreate_MissingKind(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "alerts", "create", "web", "--name", "x"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--kind is required") {
		t.Errorf("stderr = %q, want a missing --kind error", stderr.String())
	}
}

func TestRun_AppsAlertsCreate_ThresholdMissingMetric(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "alerts", "create", "web", "--name", "x", "--kind", "threshold", "--comparator", ">"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--metric is required") {
		t.Errorf("stderr = %q, want a missing --metric error", stderr.String())
	}
}

func TestRun_AppsAlertsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/alerts" {
			t.Errorf("path = %q, want /api/v1/apps/web/alerts", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]alertRuleResource{
			{ID: "alr_1", Name: "cert watch", Kind: "cert_expiry", Enabled: true, Firing: true},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "alerts", "list", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "cert watch") || !strings.Contains(stdout.String(), "cert_expiry") {
		t.Errorf("stdout = %q, want the cert_expiry rule listed", stdout.String())
	}
}

func TestRun_AppsAlertsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "alerts", "delete", "web", "alr_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/alerts/alr_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/alerts/alr_1", gotPath)
	}
	if !strings.Contains(stdout.String(), `alert rule "alr_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout.String())
	}
}
