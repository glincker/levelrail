package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func rotationEnv(url string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		switch k {
		case envAPIURL:
			return url, true
		case envAPIToken:
			return "tok", true
		}
		return "", false
	}
}

func TestAppsSecretsRotationGuards(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tests := []struct {
		name string
		args []string
	}{
		{"rotate needs yes", []string{"rotate", "web", "JWT_SECRET"}},
		{"set needs interval", []string{"set", "web", "JWT_SECRET"}},
		{"unknown verb", []string{"explode"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := runAppsSecretsRotation("levelrail", tt.args, &out, &errb, rotationEnv(srv.URL))
			if code != exitUsage {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, exitUsage, errb.String())
			}
		})
	}
	if calls != 0 {
		t.Fatalf("guarded commands made %d API calls", calls)
	}
}

func TestAppsSecretsRotationListRendersWithoutValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/secret-rotation" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[{"key":"JWT_SECRET","interval_days":30,"grace_hours":0,"owner":"ops","auto_rotate":true,"status":"overdue","state":"idle","next_due_at":"2026-01-01T00:00:00Z"}]`))
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	code := runAppsSecretsRotation("levelrail", []string{"list", "web"}, &out, &errb, rotationEnv(srv.URL))
	if code != exitOK {
		t.Fatalf("exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "JWT_SECRET") || !strings.Contains(out.String(), "overdue") {
		t.Fatalf("output = %q", out.String())
	}
}
