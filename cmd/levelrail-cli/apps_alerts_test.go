package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// testAppsAlertsCreateSimpleKind runs "apps alerts create web" for an alert
// kind that needs only --name and --kind, and asserts the request body and
// creation confirmation.
func testAppsAlertsCreateSimpleKind(t *testing.T, kind, name, id string) {
	t.Helper()
	var gotBody createAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(alertRuleResource{ID: id, Name: gotBody.Name, Kind: gotBody.Kind, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"apps", "alerts", "create", "web",
		"--name", name, "--kind", kind,
		"--api-url", srv.URL,
	})
	if gotBody.Kind != kind {
		t.Errorf("request body Kind = %q, want %s", gotBody.Kind, kind)
	}
	wantMsg := fmt.Sprintf(`alert rule %q (id %s, kind %s) created for app "web"`, name, id, kind)
	if !strings.Contains(stdout, wantMsg) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout)
	}
}

func TestRun_AppsAlertsCreate_CertExpiry_NoExtraFieldsRequired(t *testing.T) {
	testAppsAlertsCreateSimpleKind(t, "cert_expiry", "cert-expiry-watch", "alr_2")
}

func TestRun_AppsAlertsCreate_PatchStatus_NoExtraFieldsRequired(t *testing.T) {
	testAppsAlertsCreateSimpleKind(t, "patch_status", "patch-status-watch", "alr_3")
}

func TestRun_AppsAlertsCreate_NodeDiskSpace_NoExtraFieldsRequired(t *testing.T) {
	testAppsAlertsCreateSimpleKind(t, "node_disk_space", "disk-space-watch", "alr_4")
}

func TestRun_AppsAlertsCreate_NodeResourceUsage_NoExtraFieldsRequired(t *testing.T) {
	testAppsAlertsCreateSimpleKind(t, "node_resource_usage", "node-load-watch", "alr_5")
}

func TestRun_AppsAlertsCreate_DomainHealth_NoExtraFieldsRequired(t *testing.T) {
	testAppsAlertsCreateSimpleKind(t, "domain_health", "domain-watch", "alr_6")
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
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []alertRuleResource{
		{ID: "alr_1", Name: "cert watch", Kind: "cert_expiry", Enabled: true, Firing: true},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "alerts", "list", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/alerts" {
		t.Errorf("path = %q, want /api/v1/apps/web/alerts", gotPath)
	}
	if !strings.Contains(stdout, "cert watch") || !strings.Contains(stdout, "cert_expiry") {
		t.Errorf("stdout = %q, want the cert_expiry rule listed", stdout)
	}
}

func TestRun_AppsAlertsUpdate_Threshold(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody updateAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(alertRuleResource{
			ID: "alr_1", Name: gotBody.Name, Kind: gotBody.Kind, Metric: gotBody.Metric,
			Comparator: gotBody.Comparator, Threshold: gotBody.Threshold, Enabled: gotBody.Enabled,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "alerts", "update", "web", "alr_1",
		"--name", "even-higher-cpu", "--kind", "threshold", "--metric", "cpu_percent", "--comparator", ">", "--threshold", "95",
		"--api-url", srv.URL, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/alerts/alr_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/alerts/alr_1", gotPath)
	}
	if gotBody.Kind != "threshold" || gotBody.Metric != "cpu_percent" || gotBody.Comparator != ">" || gotBody.Threshold != 95 {
		t.Errorf("request body = %+v, want a threshold rule on cpu_percent > 95", gotBody)
	}
	if !strings.Contains(stdout.String(), `"id": "alr_1"`) {
		t.Errorf("stdout = %q, want the updated rule as JSON", stdout.String())
	}
}

func TestRun_AppsAlertsUpdate_MissingKind(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "alerts", "update", "web", "alr_1", "--name", "x"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--kind is required") {
		t.Errorf("stderr = %q, want a missing --kind error", stderr.String())
	}
}

func TestRun_AppsAlertsUpdate_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "alerts", "update", "web", "alr_1", "--kind", "cert_expiry"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a missing --name error", stderr.String())
	}
}

func TestRun_AppsAlertsUpdate_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "alerts", "update", "web", "--name", "x", "--kind", "cert_expiry"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
}

func TestRun_AppsAlertsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "alerts", "delete", "web", "alr_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/alerts/alr_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/apps/web/alerts/alr_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `alert rule "alr_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout)
	}
}
