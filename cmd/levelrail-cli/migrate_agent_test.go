package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTargetStat(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"vol=3:90", false},
		{"app-x-data=0:0", false},
		{"vol", true},
		{"vol=3", true},
		{"=3:90", true},
		{"vol=a:b", true},
		{"vol=-1:5", true},
	}
	for _, tc := range cases {
		_, err := parseTargetStat(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseTargetStat(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestMigrateAgentCommands(t *testing.T) {
	var paths []string
	var gotVerifyBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/migration/agent/sessions":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ms_1","token":"mig_ms_1.xyz","expires_at":"2026-10-21T10:00:00Z","ca_fingerprint":"ab","control_plane_addr":"h:1","binary":"b-migrate","command":"sudo run","session_max_hours":72}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/verify"):
			raw, _ := io.ReadAll(r.Body)
			gotVerifyBody = string(raw)
			_, _ = w.Write([]byte(`{"comparisons":[{"name":"v","status":"match"}],"advance":{"advanced":true},"databases_on_source":2,"checklist":{"app":"a"}}`))
		case strings.Contains(r.URL.Path, "/steps/data_verified"):
			_, _ = w.Write([]byte(`{"app":"a","current":"data_verified"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	env := func(k string) (string, bool) {
		switch k {
		case "APP_API_URL":
			return srv.URL, true
		case "APP_API_TOKEN":
			return "t", true
		}
		return "", false
	}
	run := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := runMigrateAgent("cli", args, &out, &errb, env)
		return code, out.String() + errb.String()
	}

	if code, out := run("start", "--label", "old"); code != exitOK || !strings.Contains(out, "sudo run") || !strings.Contains(out, "once") {
		t.Fatalf("start: %d %s", code, out)
	}
	if code, out := run("verify", "ms_1", "a", "--target", "v=1:2"); code != exitOK || !strings.Contains(out, "completed by the migration agent") || !strings.Contains(out, "2 database") {
		t.Fatalf("verify: %d %s", code, out)
	}
	if code, out := run("verify", "ms_1", "a", "--ack-empty"); code != exitOK || !strings.Contains(gotVerifyBody, `"ack_empty_volumes":true`) || strings.Contains(gotVerifyBody, "target_stats") {
		t.Fatalf("verify --ack-empty: %d %s body=%s", code, out, gotVerifyBody)
	}
	if code, out := run("verify", "ms_1", "a", "--target", "v=1:2"); code != exitOK || !strings.Contains(gotVerifyBody, `"target_stats"`) || strings.Contains(gotVerifyBody, "ack_empty_volumes") {
		t.Fatalf("verify --target: %d %s body=%s", code, out, gotVerifyBody)
	}
	if code, out := run("reopen", "a"); code != exitOK || !strings.Contains(out, "reopened") {
		t.Fatalf("reopen: %d %s", code, out)
	}
	if code, out := run("remove", "ms_1"); code != exitOK || !strings.Contains(out, "revoked") {
		t.Fatalf("remove: %d %s", code, out)
	}
	if code, _ := run("verify", "ms_1"); code != exitUsage {
		t.Fatalf("verify without an app should be a usage error, got %d", code)
	}
	if code, _ := run("nope"); code != exitUsage {
		t.Fatalf("unknown command should be a usage error, got %d", code)
	}
	want := "DELETE /api/v1/migration/agent/sessions/ms_1"
	if paths[len(paths)-1] != want {
		t.Fatalf("last call %q want %q", paths[len(paths)-1], want)
	}
}
