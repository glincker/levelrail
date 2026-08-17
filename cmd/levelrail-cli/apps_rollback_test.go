package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_AppsRollback exercises "apps rollback <name> --image <ref>" end to
// end against a fake control plane. Rollback has no dedicated backend
// endpoint (see apps_rollback.go's own doc comment): it sends the exact same
// POST /api/v1/apps/{name}/deploys request "apps deploy" does, an older,
// already-known image tag standing in for a newer one, so this test asserts
// the request that lands on the wire is indistinguishable from a deploy's,
// only the CLI's own framing (usage text, human-mode message) differs.
func TestRun_AppsRollback(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody deployTriggerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: gotBody.Image, Port: 3000})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "rollback", "web", "--image", "levelrail/web:old", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/deploys" {
		t.Errorf("path = %q, want /api/v1/apps/web/deploys (the same endpoint apps deploy uses)", gotPath)
	}
	if gotBody.Image != "levelrail/web:old" {
		t.Errorf("request body Image = %q, want %q", gotBody.Image, "levelrail/web:old")
	}
	if !strings.Contains(stdout.String(), `"image": "levelrail/web:old"`) {
		t.Errorf("stdout = %q, want the rolled-back app as JSON", stdout.String())
	}
}

func TestRun_AppsRollback_Human(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "levelrail/web:old", Port: 3000})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "rollback", "web", "--image", "levelrail/web:old", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "name:     web") {
		t.Errorf("stdout = %q, want the human app summary", stdout.String())
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "roll") {
		t.Errorf("stderr = %q, want a rollback-framed diagnostic message", stderr.String())
	}
}

func TestRun_AppsRollback_MissingImage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "rollback", "web"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--image is required") {
		t.Errorf("stderr = %q, want a missing --image validation error", stderr.String())
	}
}

func TestRun_AppsRollback_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "rollback", "--image", "levelrail/web:old"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsRollback_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "rollback", "ghost", "--image", "levelrail/web:old", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "app not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsRollback_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "rollback", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps rollback") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
