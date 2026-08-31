package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsPromote_Preview(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(promotePreviewResource{
			SourceApp:   "web-staging",
			TargetApp:   "web-prod",
			Environment: environmentResource{ID: "env_prod", Name: "production"},
			From:        promotePreviewSide{AppName: "web-staging", Image: "levelrail/web:2"},
			To:          promotePreviewSide{AppName: "web-prod", Image: "levelrail/web:1"},
			Changes: []deployCompareField{
				{Field: "image", From: "levelrail/web:1", To: "levelrail/web:2"},
			},
			UnsnapshottedFields: []string{"env", "resources"},
			Note:                "only the image tag is compared",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "promote", "web-staging", "--to", "env_prod", "--preview", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web-staging/promote/preview?to=env_prod" {
		t.Errorf("path = %q, want to query param, no mutation call", gotPath)
	}
	if !strings.Contains(stdout, "web-staging") || !strings.Contains(stdout, "web-prod") {
		t.Errorf("stdout = %q, want both app names", stdout)
	}
	if !strings.Contains(stdout, "not compared") {
		t.Errorf("stdout = %q, want the honest limitation note", stdout)
	}
}

func TestRun_AppsPromote_PreviewWithTarget(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(promotePreviewResource{
			SourceApp: "web-staging", TargetApp: "web-prod",
			Environment:         environmentResource{ID: "env_prod", Name: "production"},
			From:                promotePreviewSide{AppName: "web-staging", Image: "levelrail/web:2"},
			To:                  promotePreviewSide{AppName: "web-prod", Image: "levelrail/web:2"},
			UnsnapshottedFields: []string{"env"},
			Note:                "only the image tag is compared",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "promote", "web-staging", "--to", "env_prod", "--target", "web-prod", "--dry-run", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web-staging/promote/preview?target=web-prod&to=env_prod" {
		t.Errorf("path = %q, want target and to query params", gotPath)
	}
	if !strings.Contains(stdout, "no change") {
		t.Errorf("stdout = %q, want the no-op change note when images already match", stdout)
	}
}

func TestRun_AppsPromote_Apply(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody promoteAppRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(appResource{Name: "web-prod", Image: "levelrail/web:2", Port: 3000})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "promote", "web-staging", "--to", "env_prod", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web-staging/promote" {
		t.Errorf("method/path = %s %s, want POST /api/v1/apps/web-staging/promote", gotMethod, gotPath)
	}
	if gotBody.To != "env_prod" || gotBody.Target != "" {
		t.Errorf("request body = %+v, want To=env_prod Target=\"\"", gotBody)
	}
	if !strings.Contains(stdout.String(), "web-prod") {
		t.Errorf("stdout = %q, want the promoted app", stdout.String())
	}
	if !strings.Contains(stderr.String(), "promoted") {
		t.Errorf("stderr = %q, want a promotion confirmation", stderr.String())
	}
}

func TestRun_AppsPromote_MissingTo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "promote", "web-staging"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--to is required") {
		t.Errorf("stderr = %q, want a missing --to validation error", stderr.String())
	}
}

func TestRun_AppsPromote_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "promote", "--to", "env_prod"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsPromote_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"multiple apps tagged with environment env_prod in this project"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "promote", "web-staging", "--to", "env_prod", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "multiple apps tagged") {
		t.Errorf("stderr = %q, want the server's disambiguation error", stderr.String())
	}
}

func TestRun_AppsPromote_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "promote", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps promote") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
