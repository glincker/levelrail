package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SystemIntegrity(t *testing.T) {
	var calls []string
	var restoreBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/restore"):
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			restoreBody = b["name"]
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"pending":{"name":"snap.db","schema_version":7},"restart":{"supported":false,"hint":"sudo systemctl restart levelrail"}}`))
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"healthy":false,"mode":"full"}`))
		default:
			_, _ = w.Write([]byte(`{"status":{"healthy":false,"error":"database file is corrupt","mode":"quick"},"candidates":[{"name":"snap.db","verified":true}]}`))
		}
	}))
	defer srv.Close()

	exec := func(args ...string) (int, string) {
		var out, errOut bytes.Buffer
		args = append(append([]string{"system", "integrity"}, args...), "--api-url", srv.URL)
		code := run("levelrail-cli-test", args, &out, &errOut, envMap())
		return code, out.String() + errOut.String()
	}

	code, out := exec()
	if code != exitOK || !strings.Contains(out, "DAMAGED: database file is corrupt") || !strings.Contains(out, "snap.db") {
		t.Fatalf("status output = %d %q", code, out)
	}
	if code, _ := exec("--check", "--full"); code != exitOK || calls[len(calls)-2] != "POST /api/v1/system/integrity/check" {
		t.Fatalf("--check must POST a check before reading the state, calls %v", calls)
	}
	prev := promptHidden
	promptHidden = func(io.Writer, string) (string, error) { return "pw", nil }
	t.Cleanup(func() { promptHidden = prev })
	code, out = exec("--restore", "latest")
	if code != exitOK || restoreBody != "" || !strings.Contains(out, "staged snap.db") || !strings.Contains(out, "sudo systemctl restart levelrail") {
		t.Fatalf("restore latest = %d %q (name sent %q)", code, out, restoreBody)
	}
	if code, _ := exec("--restore", "snap.db"); code != exitOK || restoreBody != "snap.db" {
		t.Fatalf("restore by name sent %q", restoreBody)
	}
}
