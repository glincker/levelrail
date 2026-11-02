package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHasMultipleStatements(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"delete from t", false},
		{"delete from t;", false},
		{"delete from t; -- done", false},
		{"delete from t; /* done */  ", false},
		{"delete from t; drop table t", true},
		{"select 'a;b'", false},
		{"select \"a;b\" from t", false},
		{"select $$a;b$$", false},
		{"select 1 /* ; */", false},
		{"select 1 -- ; x\n", false},
		{"update t set a = 1; update t set b = 2;", true},
	}
	for _, c := range cases {
		if got := hasMultipleStatements(c.sql); got != c.want {
			t.Errorf("hasMultipleStatements(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}

func TestRun_DatabasesQuery_WriteRejectsMultipleStatements(t *testing.T) {
	srv, path, _ := newQueryEchoServer(t, `{"columns":[],"rows":[],"row_count":0}`)
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"databases", "query", "main", "--sql", "delete from t; drop table t", "--write", "--confirm", "main", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code == exitOK || !strings.Contains(stderr.String(), "exactly one statement") {
		t.Fatalf("code = %d stderr = %q, want a one-statement error", code, stderr.String())
	}
	if *path != "" {
		t.Errorf("request must not be sent for multi-statement input, got %s", *path)
	}
}

func TestRun_TokensRevoke_WithBearerToken(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	env := func(k string) (string, bool) {
		switch k {
		case envLevelrailToken:
			return "env-tok", true
		case "CI":
			return "true", true
		}
		return "", false
	}
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"tokens", "revoke", "tok_1", "--api-url", srv.URL}, &stdout, &stderr, env)
	if code != exitOK {
		t.Fatalf("exit = %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	if gotAuth != "Bearer env-tok" || gotPath != "/api/v1/auth/tokens/tok_1" {
		t.Errorf("auth = %q path = %q", gotAuth, gotPath)
	}
}

func TestRun_TokensRevoke_BearerRefusedInCIModeNamesFix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"authentication required"}`))
	}))
	t.Cleanup(srv.Close)
	env := func(k string) (string, bool) {
		switch k {
		case envLevelrailToken:
			return "env-tok", true
		case "CI":
			return "true", true
		}
		return "", false
	}
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"tokens", "revoke", "tok_1", "--api-url", srv.URL}, &stdout, &stderr, env)
	if code == exitOK || !strings.Contains(stderr.String(), "--username and --password") {
		t.Fatalf("code = %d stderr = %q, want the session fix named", code, stderr.String())
	}
}

func TestHelpNamesBothTokenEnvVars(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run("levelrail-cli-test", []string{"apps", "deploy", "-h"}, &stdout, &stderr, envMap())
	out := stdout.String() + stderr.String()
	if !strings.Contains(out, apiTokenEnvHelp) {
		t.Errorf("help = %q, want %q", out, apiTokenEnvHelp)
	}
}

func backupTargetsFake(t *testing.T, history []BackupHistoryForTest, deleted *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/backups", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("before") != "" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_ = json.NewEncoder(w).Encode(history)
	})
	mux.HandleFunc("DELETE /api/v1/backup-targets/{id}", func(w http.ResponseWriter, r *http.Request) {
		*deleted = r.PathValue("id")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type BackupHistoryForTest struct {
	ID        string `json:"id"`
	TargetID  string `json:"target_id"`
	StartedAt string `json:"started_at"`
}

func TestRun_BackupsTargetsDelete(t *testing.T) {
	var deleted string
	srv := backupTargetsFake(t, nil, &deleted)
	stdout, _ := runCLIExpectOK(t, []string{"backups", "targets", "delete", "bkt_1", "--api-url", srv.URL})
	if deleted != "bkt_1" || !strings.Contains(stdout, "bkt_1") {
		t.Errorf("deleted = %q stdout = %q", deleted, stdout)
	}
}

func TestRun_BackupsTargetsDelete_HistoryNeedsYes(t *testing.T) {
	var deleted string
	srv := backupTargetsFake(t, []BackupHistoryForTest{{ID: "h1", TargetID: "bkt_1", StartedAt: "2026-10-01T00:00:00Z"}}, &deleted)
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"backups", "targets", "delete", "bkt_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code == exitOK || !strings.Contains(stderr.String(), "--yes") || deleted != "" {
		t.Fatalf("code = %d stderr = %q deleted = %q, want refusal naming --yes", code, stderr.String(), deleted)
	}
	runCLIExpectOK(t, []string{"backups", "targets", "delete", "bkt_1", "--yes", "--api-url", srv.URL})
	if deleted != "bkt_1" {
		t.Errorf("deleted = %q after --yes", deleted)
	}
}

func TestRun_AppsDeploy_NoImageExplainsNextCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"web","image":"web:pending","port":80}`))
	}))
	t.Cleanup(srv.Close)
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "deploy", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	got := stderr.String()
	if code == exitOK || !strings.Contains(got, "is stopped") || !strings.Contains(got, "apps deploy web --image IMAGE") || !strings.Contains(got, " up") {
		t.Fatalf("code = %d stderr = %q, want the missing image and next commands", code, got)
	}
}

func TestRun_AppsDeploy_WaitFlagPointsToDeploysWait(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "deploy", "web", "--wait"}, &stdout, &stderr, envMap())
	if code != exitUsage || !strings.Contains(stderr.String(), "apps deploys wait") {
		t.Fatalf("code = %d stderr = %q", code, stderr.String())
	}
}

func TestRun_AppsCreate_ImageSaysStoppedUntilDeployed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write([]byte(`{"name":"web","image":"nginx:1","port":80}`))
	}))
	t.Cleanup(srv.Close)
	_, stderr := runCLIExpectOK(t, []string{"apps", "create", "--name", "web", "--image", "nginx:1", "--port", "80", "--api-url", srv.URL})
	if !strings.Contains(stderr, "stays stopped until it is deployed") || !strings.Contains(stderr, "apps deploy web --image IMAGE") {
		t.Errorf("stderr = %q, want the stopped note and deploy command", stderr)
	}
}
