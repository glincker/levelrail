package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_IAMSuggest(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"window_days":14,"note":"n","tokens":[{"token_id":"t1","name":"ci","granted":["root"],"used":[],"unused":["root"],"suggested_abilities":["read","write"],"status":"narrow","risk":"high","reason":"r"}]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "suggest", "--api-url", srv.URL, "--days", "14"})
	if gotQuery != "/api/v1/iam/suggestions?days=14" {
		t.Errorf("request = %q", gotQuery)
	}
	if !strings.Contains(stdout, "narrow") || !strings.Contains(stdout, "read,write") {
		t.Errorf("stdout = %q", stdout)
	}
}
