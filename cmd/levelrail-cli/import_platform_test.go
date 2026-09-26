package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func importEnv(vals map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := vals[k]
		return v, ok
	}
}

func newImportAPI(t *testing.T, counts map[string]int) (*httptest.Server, *[]string, *apiclient.PlatformImportRequest) {
	t.Helper()
	var paths []string
	var got apiclient.PlatformImportRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.PlatformImportReport{Platform: "coolify", Counts: counts, Notes: []string{"Databases are created empty."},
			Items: []apiclient.PlatformImportItem{{Kind: "app", SourceID: "1", SourceName: "web", Target: "web", Status: "needs-attention", Reasons: []string{"port defaulted"}, Manual: []string{"set the port"}}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &paths, &got
}

func TestImportPlatformDryRunUsesDiscover(t *testing.T) {
	srv, paths, got := newImportAPI(t, map[string]int{"needs-attention": 1})
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "platform", "coolify", "--url", "https://c.example.com", "--dry-run", "--only", "a, b", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{envImportSourceToken: "tok-env", "APP_API_TOKEN": "cp-token"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if len(*paths) != 1 || (*paths)[0] != "/api/v1/imports/platform/discover" {
		t.Errorf("paths: %v", *paths)
	}
	if got.Token != "tok-env" || got.Platform != "coolify" || len(got.Only) != 2 || got.Only[1] != "b" || got.Collision != "suffix" {
		t.Errorf("request: %+v", got)
	}
	for _, want := range []string{"Dry run", "web", "port defaulted", "next: set the port", "created empty"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "tok-env") || strings.Contains(errb.String(), "tok-env") {
		t.Error("token must never be printed")
	}
}

func TestImportPlatformApplyStdinTokenAndFailureExit(t *testing.T) {
	srv, paths, got := newImportAPI(t, map[string]int{"failed": 1})
	var out, errb bytes.Buffer
	code := runImportPlatform("cli", []string{"dokploy", "--url", "https://d.example.com", "--token-stdin", "--json", "--api-url", srv.URL},
		strings.NewReader("tok-stdin\nignored"), &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp-token"}))
	if code != exitCheckFailed {
		t.Fatalf("exit %d, want failure exit: %s", code, errb.String())
	}
	if (*paths)[0] != "/api/v1/imports/platform/apply" || got.Token != "tok-stdin" {
		t.Errorf("paths=%v token=%q", *paths, got.Token)
	}
	var rep apiclient.PlatformImportReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || rep.Counts["failed"] != 1 {
		t.Errorf("json output: %v %s", err, out.String())
	}
}

func TestImportPlatformTokenFlagWarnsAndValidation(t *testing.T) {
	srv, _, _ := newImportAPI(t, map[string]int{})
	var out, errb bytes.Buffer
	code := runImportPlatform("cli", []string{"caprover", "--url", "https://x", "--token", "pw", "--dry-run", "--api-url", srv.URL},
		strings.NewReader(""), &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp-token"}))
	if code != exitOK || !strings.Contains(errb.String(), "shell history") {
		t.Errorf("want warning, exit %d: %s", code, errb.String())
	}
	cases := [][]string{
		{"nope", "--url", "https://x"},
		{"coolify"},
		{"coolify", "--url", "https://x"},
	}
	for _, args := range cases {
		var o, e bytes.Buffer
		if c := runImportPlatform("cli", args, strings.NewReader(""), &o, &e, importEnv(nil)); c != exitUsage {
			t.Errorf("%v: exit %d, want usage: %s", args, c, e.String())
		}
	}
	var o, e bytes.Buffer
	if c := runImport("cli", []string{"repo"}, &o, &e, importEnv(nil)); c != exitUsage {
		t.Errorf("import repo: exit %d", c)
	}
}
