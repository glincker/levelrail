package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsList_Unpaged_BareArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("limit") != "" || q.Get("offset") != "" {
			t.Errorf("unpaged request should carry no limit/offset, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appResource{{Name: "web"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "list", "--api-url", srv.URL, "--json"})
	if strings.Contains(stdout, "next_token") || strings.Contains(stdout, "items") {
		t.Errorf("stdout = %q, want the plain historical array shape, not the paged wrapper", stdout)
	}
	if !strings.Contains(stdout, `"name": "web"`) {
		t.Errorf("stdout = %q, want the app listed", stdout)
	}
}

func TestRun_AppsList_MaxItems_WiresLimitAndReturnsNextToken(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "3")
		_ = json.NewEncoder(w).Encode([]appResource{{Name: "web"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "list", "--api-url", srv.URL, "--max-items", "1", "--json"})
	if !strings.Contains(gotQuery, "limit=1") {
		t.Errorf("request query = %q, want limit=1", gotQuery)
	}
	if !strings.Contains(stdout, `"next_token": "1"`) {
		t.Errorf("stdout = %q, want next_token \"1\" (offset+len(items), more remain per X-Total-Count)", stdout)
	}
	if !strings.Contains(stdout, `"total_count": 3`) {
		t.Errorf("stdout = %q, want total_count 3", stdout)
	}
}

func TestRun_AppsList_StartingToken_WiresOffset(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "2")
		_ = json.NewEncoder(w).Encode([]appResource{{Name: "api"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "list", "--api-url", srv.URL, "--max-items", "1", "--starting-token", "1", "--json"})
	if !strings.Contains(gotQuery, "limit=1") || !strings.Contains(gotQuery, "offset=1") {
		t.Errorf("request query = %q, want limit=1 and offset=1", gotQuery)
	}
	if !strings.Contains(stdout, `"next_token": ""`) {
		t.Errorf("stdout = %q, want an empty next_token (no more items past total_count)", stdout)
	}
}

func TestRun_AppsList_StartingToken_WithoutMaxItems_IsValidationError(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"apps", "list", "--api-url", "http://unused.invalid", "--starting-token", "5"})
	if !strings.Contains(stderr, "--starting-token requires --max-items") {
		t.Errorf("stderr = %q, want the --max-items requirement explained", stderr)
	}
}

func TestRun_AppsList_StartingToken_NotANumber_IsValidationError(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"apps", "list", "--api-url", "http://unused.invalid", "--max-items", "10", "--starting-token", "not-a-number"})
	if !strings.Contains(stderr, "must be a non-negative integer offset") {
		t.Errorf("stderr = %q, want the offset-format error", stderr)
	}
}
