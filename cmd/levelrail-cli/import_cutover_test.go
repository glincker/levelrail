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

func cutoverAPI(t *testing.T, finalState string) (*httptest.Server, *[]string, *map[string]any) {
	t.Helper()
	var calls []string
	var started map[string]any
	run := map[string]any{"id": "cut_1", "app": "web", "mode": "switch", "state": "starting",
		"domains": []any{map[string]any{"domain": "web.example.com", "method": "dns",
			"previous": []any{map[string]any{"name": "web", "type": "A", "value": "203.0.113.9"}}}},
		"steps": []any{map[string]any{"name": "route", "state": "done", "detail": "started", "duration_ms": 12}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/cutover/plan"):
			_ = json.NewEncoder(w).Encode(map[string]any{"app": "web", "verdict": "warnings",
				"checks": []any{map[string]any{"id": "volumes", "title": "Volumes copied", "status": "warn", "detail": "1 volume pending",
					"fix": map[string]any{"summary": "Copy the volume first."}}},
				"domains": []any{map[string]any{"domain": "web.example.com", "method": "dns", "current": []any{"203.0.113.9"},
					"desired": map[string]any{"type": "A", "value": "198.51.100.4"}}}})
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/cutover/runs"):
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &started)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(run)
		case strings.HasSuffix(p, "/rollback"):
			run["state"] = "rolled_back"
			_ = json.NewEncoder(w).Encode(run)
		case strings.HasSuffix(p, "/cutover/runs"):
			_ = json.NewEncoder(w).Encode(map[string]any{"runs": []any{run}})
		default:
			run["state"] = finalState
			_ = json.NewEncoder(w).Encode(run)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &started
}

func TestImportCutoverPlanPrintsChecklist(t *testing.T) {
	srv, _, _ := cutoverAPI(t, "live")
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "cutover", "plan", "--session", "appimp-1", "--app", "web", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, want := range []string{"warnings", "Volumes copied", "Copy the volume first.", "web.example.com via dns", "A 198.51.100.4"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q: %s", want, out.String())
		}
	}
}

func TestImportCutoverRunNeedsTypedConfirmation(t *testing.T) {
	srv, calls, _ := cutoverAPI(t, "live")
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "cutover", "run", "--session", "appimp-1", "--app", "web", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitUsage || len(*calls) != 0 {
		t.Fatalf("exit %d calls %v", code, *calls)
	}
}

func TestImportCutoverRunDryRunAndNoWait(t *testing.T) {
	srv, calls, started := cutoverAPI(t, "ready")
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "cutover", "run", "--session", "appimp-1", "--app", "web", "--dry-run", "--no-wait", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if (*started)["mode"] != "dry_run" || len(*calls) != 1 {
		t.Fatalf("started = %v calls = %v", *started, *calls)
	}
	if !strings.Contains(out.String(), "cut_1") || !strings.Contains(out.String(), "203.0.113.9") {
		t.Errorf("stdout: %s", out.String())
	}
}

func TestImportCutoverStatusAndRollbackUseLatestRun(t *testing.T) {
	srv, calls, _ := cutoverAPI(t, "live")
	env := importEnv(map[string]string{"APP_API_TOKEN": "cp"})
	var out, errb bytes.Buffer
	if code := run("cli", []string{"import", "cutover", "rollback", "--session", "appimp-1", "--app", "web", "--api-url", srv.URL}, &out, &errb, env); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	got := strings.Join(*calls, "\n")
	if !strings.Contains(got, "GET /api/v1/migration/apps/sessions/appimp-1/items/web/cutover/runs") ||
		!strings.Contains(got, "POST /api/v1/migration/apps/sessions/appimp-1/items/web/cutover/runs/cut_1/rollback") {
		t.Fatalf("calls: %s", got)
	}
	if !strings.Contains(out.String(), "rolled_back") {
		t.Errorf("stdout: %s", out.String())
	}
}

func TestImportCutoverUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"import", "cutover"}, {"import", "cutover", "plan"}, {"import", "cutover", "nope", "--session", "s", "--app", "a"}} {
		var out, errb bytes.Buffer
		if c := run("cli", args, &out, &errb, importEnv(map[string]string{})); c != exitUsage {
			t.Errorf("%v: exit %d, want usage", args, c)
		}
	}
}
