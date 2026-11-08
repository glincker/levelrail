package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type reqRecord struct{ method, path, body string }

func newRecordingServer(t *testing.T, response string) (*httptest.Server, *[]reqRecord) {
	t.Helper()
	var got []reqRecord
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, reqRecord{r.Method, r.URL.EscapedPath(), string(b)})
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestRun_DatabasesEngineStats_StripsControlChars(t *testing.T) {
	srv, got := newRecordingServer(t, `{"engine":"meilisearch","objects":[{"name":"idx\u001b[2Jx","count":7}],"total":7}`)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "engine-stats", "s", "--api-url", srv.URL})
	if (*got)[0].method != "GET" || (*got)[0].path != "/api/v1/databases/s/engine-stats" {
		t.Errorf("request = %+v", (*got)[0])
	}
	if strings.ContainsRune(stdout, 0x1b) || !strings.Contains(stdout, "idx?[2Jx") || !strings.Contains(stdout, "total: 7") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_DatabasesQueryHistory_ListAndClear(t *testing.T) {
	srv, got := newRecordingServer(t, `[{"id":"h1","mode":"read","sql":"select\n  'secret-literal-0123456789' from t","ok":true,"duration_ms":3,"row_count":1,"created_at":"2026-11-08T10:00:00Z"}]`)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "query-history", "list", "main", "--width", "20", "--api-url", srv.URL})
	if !strings.Contains(stdout, "...") || strings.Contains(stdout, "0123456789") {
		t.Errorf("sql must be one truncated line: %q", stdout)
	}
	n := len(*got)
	stdout, _ = runCLIExpectOK(t, []string{"databases", "query-history", "clear", "main", "--api-url", srv.URL})
	if len(*got) != n || !strings.Contains(stdout, "--yes") {
		t.Fatalf("clear without --yes must send nothing: %+v %q", *got, stdout)
	}
	runCLIExpectOK(t, []string{"databases", "query-history", "clear", "a/b", "--yes", "--api-url", srv.URL})
	last := (*got)[len(*got)-1]
	if last.method != "DELETE" || last.path != "/api/v1/databases/a%2Fb/query-history" {
		t.Errorf("last = %+v", last)
	}
}

func TestRun_DatabasesSavedQueries_AddAndDelete(t *testing.T) {
	srv, got := newRecordingServer(t, `{"id":"q1","name":"n","sql":"select 1","created_at":"2026-11-08T10:00:00Z"}`)
	runCLIExpectOK(t, []string{"databases", "saved-queries", "add", "main", "--name", "n", "--sql", "select 1", "--api-url", srv.URL})
	r := (*got)[0]
	if r.method != "POST" || r.path != "/api/v1/databases/main/saved-queries" || !strings.Contains(r.body, `"name":"n"`) || !strings.Contains(r.body, `"sql":"select 1"`) {
		t.Errorf("add request = %+v", r)
	}
	big := strings.Repeat("a", savedQueryMaxBytes+1)
	if _, err := resolveSavedSQL(big, ""); err == nil {
		t.Error("oversize sql must be rejected before any request")
	}
	n := len(*got)
	runCLIExpectOK(t, []string{"databases", "saved-queries", "delete", "main", "q1", "--api-url", srv.URL})
	if len(*got) != n {
		t.Fatal("delete without --yes must send nothing")
	}
	runCLIExpectOK(t, []string{"databases", "saved-queries", "delete", "main", "q/1", "--yes", "--api-url", srv.URL})
	last := (*got)[len(*got)-1]
	if last.method != "DELETE" || last.path != "/api/v1/databases/main/saved-queries/q%2F1" {
		t.Errorf("last = %+v", last)
	}
}
