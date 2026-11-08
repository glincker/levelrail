package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_MailBroadcasts_SendDryRunAndCancel(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			_, _ = w.Write([]byte(`{"id":"mb_1","status":"cancelled"}`))
			return
		}
		_, _ = w.Write([]byte(`{"total":3,"deliverable":2,"suppressed":1,"cap_remaining":97}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"mail", "broadcasts", "send", "--subject", "Hi", "--body", "Hello", "--to", "a@example.com", "--dry-run", "--api-url", srv.URL})
	if gotPath != "POST /api/v1/mail/broadcasts" || gotBody["dry_run"] != true || !strings.Contains(stdout, "2 deliverable") {
		t.Errorf("path %q body %v stdout %q", gotPath, gotBody, stdout)
	}

	stdout, _ = runCLIExpectOK(t, []string{"mail", "broadcasts", "cancel", "mb_1", "--api-url", srv.URL})
	if gotPath != "POST /api/v1/mail/broadcasts/mb_1/cancel" || !strings.Contains(stdout, "cancelled") {
		t.Errorf("path %q stdout %q", gotPath, stdout)
	}
}
