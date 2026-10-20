package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func takeoverAPI(t *testing.T) (*httptest.Server, *[]string, *map[string]any) {
	t.Helper()
	var calls []string
	var started map[string]any
	run := map[string]any{"id": "tk-1", "state": "done", "rollbackable": true,
		"steps":  []any{map[string]any{"id": "stop_old_proxy", "state": "done", "detail": "stopped, not removed"}},
		"verify": []any{map[string]any{"domain": "web.example.com", "ok": true}}}
	plan := map[string]any{"verdict": "warnings", "current_http": ":8088", "current_https": ":8443", "target_http": ":80", "target_https": ":443",
		"checks": []any{map[string]any{"id": "certificates", "title": "TLS certificates", "status": "warn", "detail": "HTTP-01 on first use",
			"fix": map[string]any{"summary": "Upload a certificate."}}},
		"steps":    []any{map[string]any{"id": "stop_old_proxy", "title": "Stop the old proxy", "detail": "Stop coolify-proxy."}},
		"rollback": []any{"Start coolify-proxy again."}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/migration/takeover/plan":
			_ = json.NewEncoder(w).Encode(map[string]any{"plan": plan})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/migration/takeover/runs":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &started)
			if started["confirm"] != true {
				_ = json.NewEncoder(w).Encode(map[string]any{"dry_run": true, "plan": plan})
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"created": true, "run": run})
		case strings.HasSuffix(r.URL.Path, "/rollback"):
			run["state"] = "rolled_back"
			_ = json.NewEncoder(w).Encode(map[string]any{"run": run})
		case r.URL.Path == "/api/v1/migration/takeover/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{"runs": []any{run}})
		default:
			_ = json.NewEncoder(w).Encode(run)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &started
}

func runTakeover(srv *httptest.Server, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	full := append([]string{"import", "takeover"}, args...)
	full = append(full, "--api-url", srv.URL)
	code := run("cli", full, &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	return code, out.String(), errb.String()
}

func TestImportTakeoverPlanPrintsChecksStepsAndRollback(t *testing.T) {
	srv, _, _ := takeoverAPI(t)
	code, out, errs := runTakeover(srv, "plan")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	for _, want := range []string{"warnings", "TLS certificates", "Upload a certificate.", "Stop coolify-proxy.", "Start coolify-proxy again."} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q: %s", want, out)
		}
	}
}

func TestImportTakeoverRunNeedsConfirmAndDryRunChangesNothing(t *testing.T) {
	srv, _, started := takeoverAPI(t)
	if code, _, errs := runTakeover(srv, "run"); code != exitUsage || !strings.Contains(errs, "--confirm takeover") {
		t.Fatalf("unconfirmed run: exit %d: %s", code, errs)
	}
	code, out, _ := runTakeover(srv, "run", "--dry-run")
	if code != exitOK || !strings.Contains(out, "nothing was changed") || (*started)["confirm"] == true {
		t.Fatalf("dry run: exit %d started=%v out=%s", code, *started, out)
	}
}

func TestImportTakeoverRunConfirmedPrintsRun(t *testing.T) {
	srv, _, started := takeoverAPI(t)
	code, out, errs := runTakeover(srv, "run", "--confirm", "takeover", "--no-wait", "--json")
	if code != exitOK || (*started)["confirm"] != true {
		t.Fatalf("exit %d started=%v: %s", code, *started, errs)
	}
	if !strings.Contains(out, `"id": "tk-1"`) && !strings.Contains(out, `"id":"tk-1"`) {
		t.Fatalf("json lacks the run: %s", out)
	}
}

func TestImportTakeoverRollbackUsesLatestRun(t *testing.T) {
	srv, calls, _ := takeoverAPI(t)
	code, out, errs := runTakeover(srv, "rollback", "--no-wait", "--reason", "testing")
	if code != exitOK || !strings.Contains(out, "rolled_back") {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	if got := strings.Join(*calls, ","); !strings.Contains(got, "POST /api/v1/migration/takeover/runs/tk-1/rollback") {
		t.Fatalf("calls = %s", got)
	}
}
