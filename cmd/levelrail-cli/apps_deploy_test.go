package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsDeploy(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody deployTriggerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: gotBody.Image, Port: 3000})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy", "web", "--image", "levelrail/web:2", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/deploys" {
		t.Errorf("request = %s %s, want POST /api/v1/apps/web/deploys", gotMethod, gotPath)
	}
	if gotBody.Image != "levelrail/web:2" {
		t.Errorf("request body Image = %q, want levelrail/web:2", gotBody.Image)
	}
	if gotBody.Confirm {
		t.Errorf("request body Confirm = true, want false (not a protected environment)")
	}
}

func TestRun_AppsDeploy_MissingImage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy", "web"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--image is required") {
		t.Errorf("stderr = %q, want a missing --image validation error", stderr.String())
	}
}

func TestRun_AppsDeploy_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy", "--image", "levelrail/web:2"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsDeploy_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps deploy") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

// protectedEnvironmentDeployServer fails every request with a 409 naming
// a protected environment until confirm: true arrives on the request
// body, then succeeds: the fake control plane behind every
// confirm-retry test in this file (deploy, rollback, promote).
func protectedEnvironmentDeployServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body deployTriggerRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if !body.Confirm {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"environment \"production\" is protected; set confirm: true to proceed"}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: body.Image, Port: 3000})
	}))
	return srv, &calls
}

// TestRun_AppsDeploy_ProtectedEnvironment_ConfirmFlagSkipsPrompt covers
// the scripted path: --confirm sends confirm: true on the very first
// call, so the server never has a reason to 409 and the prompt is never
// reached.
func TestRun_AppsDeploy_ProtectedEnvironment_ConfirmFlagSkipsPrompt(t *testing.T) {
	srv, calls := protectedEnvironmentDeployServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := runAppsDeploy("levelrail-cli-test", []string{"web", "--image", "levelrail/web:2", "--confirm", "--api-url", srv.URL}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry needed)", *calls)
	}
}

// TestRun_AppsDeploy_ProtectedEnvironment_InteractivePromptConfirms
// covers the interactive path: no --confirm, the first call 409s, the
// prompt reads "yes" from stdin, and the retry with confirm: true
// succeeds.
func TestRun_AppsDeploy_ProtectedEnvironment_InteractivePromptConfirms(t *testing.T) {
	srv, calls := protectedEnvironmentDeployServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := runAppsDeploy("levelrail-cli-test", []string{"web", "--image", "levelrail/web:2", "--api-url", srv.URL}, &stdout, &stderr, envMap(), strings.NewReader("yes\n"))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if *calls != 2 {
		t.Errorf("calls = %d, want 2 (blocked, then confirmed retry)", *calls)
	}
	if !strings.Contains(stderr.String(), "protected") {
		t.Errorf("stderr = %q, want the protected-environment prompt", stderr.String())
	}
}

// TestRun_AppsDeploy_ProtectedEnvironment_DeclinedNeverRetries covers
// declining the interactive prompt: the server's original 409 is
// reported, and no second, confirmed call is ever made.
func TestRun_AppsDeploy_ProtectedEnvironment_DeclinedNeverRetries(t *testing.T) {
	srv, calls := protectedEnvironmentDeployServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := runAppsDeploy("levelrail-cli-test", []string{"web", "--image", "levelrail/web:2", "--api-url", srv.URL}, &stdout, &stderr, envMap(), strings.NewReader("no\n"))
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want 1 (declining must not retry)", *calls)
	}
	if !strings.Contains(stderr.String(), "protected") {
		t.Errorf("stderr = %q, want the protected-environment error surfaced", stderr.String())
	}
}

// TestRun_AppsDeploy_ProtectedEnvironment_EOFRefusesCleanly: no
// --confirm and no terminal attached (stdin at EOF) must refuse rather
// than hang, mirroring backups_restore.go's own EOF handling.
func TestRun_AppsDeploy_ProtectedEnvironment_EOFRefusesCleanly(t *testing.T) {
	srv, calls := protectedEnvironmentDeployServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := runAppsDeploy("levelrail-cli-test", []string{"web", "--image", "levelrail/web:2", "--api-url", srv.URL}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want 1 (EOF must not retry)", *calls)
	}
}
