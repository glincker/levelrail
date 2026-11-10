package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func composeRestoreServer(t *testing.T, restores *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm string `json:"confirm"`
			Preview bool   `json:"preview"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Preview {
			*restores++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"run_id":"csr_1","object_key":"k","size_bytes":3,"engine":"postgres","app":"shop","service":"db","confirm_phrase":"restore shop/db","preview":` + map[bool]string{true: "true", false: "false"}[body.Preview] + `,"safety_run_id":"csr_s"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestComposeBackupsRestore_NoYesNonTTYSendsNoRestore(t *testing.T) {
	restores := 0
	srv := composeRestoreServer(t, &restores)
	var stdout, stderr bytes.Buffer
	code := runAppsBackupsRestore("levelrail-cli-test", []string{"--api-url", srv.URL, "--token", "x", "shop", "db", "csr_1"}, &stdout, &stderr, envMap(), strings.NewReader("restore shop/db\n"))
	if code == exitOK || restores != 0 {
		t.Fatalf("code = %d restores = %d, want refusal and zero restore requests", code, restores)
	}
}

func TestComposeBackupsRestore_YesSendsConfirmedRestore(t *testing.T) {
	restores := 0
	srv := composeRestoreServer(t, &restores)
	var stdout, stderr bytes.Buffer
	code := runAppsBackupsRestore("levelrail-cli-test", []string{"--api-url", srv.URL, "--token", "x", "--yes", "shop", "db", "csr_1"}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if code != exitOK || restores != 1 {
		t.Fatalf("code = %d restores = %d stderr = %s", code, restores, stderr.String())
	}
}
