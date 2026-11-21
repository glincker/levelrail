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

	stdout, stderr := runCLIExpectOK(t, []string{"apps", "list", "--api-url", srv.URL, "--max-items", "1", "--json"})
	if !strings.Contains(gotQuery, "limit=1") {
		t.Errorf("request query = %q, want limit=1", gotQuery)
	}
	if !strings.Contains(stderr, "next_token=1 total_count=3") {
		t.Errorf("stderr = %q, want next_token=1 total_count=3", stderr)
	}
	if strings.Contains(stdout, "next_token") || !strings.HasPrefix(strings.TrimSpace(stdout), "[") {
		t.Errorf("stdout = %q, want a bare JSON array", stdout)
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

	_, stderr := runCLIExpectOK(t, []string{"apps", "list", "--api-url", srv.URL, "--max-items", "1", "--starting-token", "1", "--json"})
	if !strings.Contains(gotQuery, "limit=1") || !strings.Contains(gotQuery, "offset=1") {
		t.Errorf("request query = %q, want limit=1 and offset=1", gotQuery)
	}
	if !strings.Contains(stderr, "next_token= total_count=2") {
		t.Errorf("stderr = %q, want an empty next_token (no more items past total_count)", stderr)
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

func TestRun_AppsList_JSON_ExposesPreviewStateAndKind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"web","app_kind":"static","preview":{"state":"rendering"}},{"name":"api"}]`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "list", "--api-url", srv.URL, "--json"})
	var got []appResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got[0].AppKind != "static" || got[0].Preview.State != "rendering" {
		t.Errorf("first row = %+v, want kind static and state rendering", got[0])
	}
	if got[1].Preview == nil || got[1].Preview.State != "none" {
		t.Errorf("second row preview = %+v, want state none", got[1].Preview)
	}
}
