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

func newQueryEchoServer(t *testing.T, response string) (*httptest.Server, *string, *string) {
	t.Helper()
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotPath, &gotBody
}

func TestRun_DatabasesQuery_ReadOnlyByDefault(t *testing.T) {
	srv, path, body := newQueryEchoServer(t, `{"columns":["id","note"],"rows":[["1",null]],"row_count":1,"duration_ms":4}`)

	stdout, _ := runCLIExpectOK(t, []string{"db", "query", "main", "--sql", "select id, note from t", "--api-url", srv.URL})
	if *path != "/api/v1/databases/main/query" {
		t.Errorf("path = %s", *path)
	}
	var req map[string]any
	_ = json.Unmarshal([]byte(*body), &req)
	if req["sql"] != "select id, note from t" {
		t.Errorf("body = %s", *body)
	}
	if !strings.Contains(stdout, "NULL") || !strings.Contains(stdout, "(1 rows, 4 ms)") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_DatabasesQuery_WriteNeedsConfirm(t *testing.T) {
	srv, path, _ := newQueryEchoServer(t, `{"columns":[],"rows":[],"row_count":0}`)

	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"databases", "query", "main", "--sql", "delete from t", "--write", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code == exitOK || !strings.Contains(stderr.String(), "--confirm main") {
		t.Fatalf("code = %d stderr = %q, want usage error naming --confirm", code, stderr.String())
	}
	if *path != "" {
		t.Errorf("request must not be sent without --confirm, got %s", *path)
	}

	runCLIExpectOK(t, []string{"databases", "query", "main", "--sql", "delete from t", "--write", "--confirm", "main", "--api-url", srv.URL})
	if *path != "/api/v1/databases/main/query/write" {
		t.Errorf("path = %s", *path)
	}
}

func TestRun_DatabasesQuery_Explain(t *testing.T) {
	srv, path, body := newQueryEchoServer(t, `{"columns":["QUERY PLAN"],"rows":[["Seq Scan on t"]],"row_count":1}`)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "query", "main", "--sql", "select 1", "--explain", "--analyze", "--api-url", srv.URL})
	if *path != "/api/v1/databases/main/explain" || !strings.Contains(*body, `"analyze":true`) {
		t.Errorf("path = %s body = %s", *path, *body)
	}
	if !strings.Contains(stdout, "Seq Scan on t") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_DatabasesSchema(t *testing.T) {
	srv, path, _ := newQueryEchoServer(t, `{"engine":"postgres","schemas":[{"name":"public","tables":[{"name":"users","kind":"table","row_estimate":42,"size_bytes":8192,"columns":[{"name":"id","type":"integer","nullable":false,"primary_key":true}],"indexes":[]}]}]}`)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "schema", "main", "--columns", "--api-url", srv.URL})
	if *path != "/api/v1/databases/main/schema" {
		t.Errorf("path = %s", *path)
	}
	for _, want := range []string{"users", "42", "8.0 KiB", "id", "PK"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}
