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

func TestMigrateDbCopyStdinPasswordAndFailureExit(t *testing.T) {
	var got apiclient.DataCopySource
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(apiclient.DataCopyStatus{Database: "main", Engine: "postgres", Status: "failed",
			Reason: "1 of 2 tables differ", Checked: 2, Mismatched: 1,
			Tables: []apiclient.DataCopyTable{{Name: "public.users", Source: 10, Target: 9}}})
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	code := runMigrateDbCopy("cli", []string{"main", "--host", "old.example.com", "--user", "u", "--database", "app", "--password-stdin", "--api-url", srv.URL},
		strings.NewReader("hunter2\n"), &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitCheckFailed {
		t.Fatalf("exit %d, want the failure exit: %s", code, errb.String())
	}
	if path != "/api/v1/imports/platform/databases/main/copy" || got.Password != "hunter2" || got.Host != "old.example.com" {
		t.Errorf("path=%s body=%+v", path, got)
	}
	for _, want := range []string{"failed", "public.users", "DIFFERS"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String()+errb.String(), "hunter2") {
		t.Error("password must never be printed")
	}
}

func TestMigrateDbCopyNeedsHost(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runMigrateDbCopy("cli", []string{"main"}, strings.NewReader(""), &out, &errb, importEnv(nil)); code != exitUsage {
		t.Fatalf("exit %d, want usage", code)
	}
}

func TestMigrateCutoverPrintsExactRecordAndExitCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/migration/cutover" || r.URL.Query().Get("target_ip") != "203.0.113.10" {
			t.Errorf("request = %s", r.URL)
		}
		_ = json.NewEncoder(w).Encode(apiclient.CutoverReport{Verdict: "wait", Phase: "pre-switch", TargetIPs: []string{"203.0.113.10"},
			Domains: []apiclient.CutoverDomain{{App: "web", Domain: "app.example.com", Verdict: "wait",
				Checks: []apiclient.CutoverCheck{{ID: "dns-ttl", Status: "warn", Detail: "TTL is 3600s", Fix: "set the TTL of app.example.com to 300"}},
				Change: &apiclient.CutoverChange{Type: "A", Name: "app.example.com", Value: "203.0.113.10", TTL: 300, Replaces: "198.51.100.7"}}}})
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	code := runMigrateCutover("cli", []string{"--target-ip", "203.0.113.10", "--api-url", srv.URL}, &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitCheckFailed {
		t.Fatalf("exit %d, want non-zero while waiting: %s", code, errb.String())
	}
	for _, want := range []string{"WAIT", "set the TTL of app.example.com to 300", "set A app.example.com -> 203.0.113.10 (TTL 300), replacing 198.51.100.7"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q: %s", want, out.String())
		}
	}
}
