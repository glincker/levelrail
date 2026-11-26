package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesMongoCollections(t *testing.T) {
	srv, got := newRecordingServer(t, `{"databases":[{"kind":"db","name":"app","sizeOnDisk":10}],"collections":[{"kind":"collection","db":"app","name":"users\u001b[2J","type":"collection","count":5,"size":100,"indexes":[{"name":"_id_"}]}],"truncated":false,"fingerprint":"f"}`)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "mongo", "collections", "m", "--database", "app", "--api-url", srv.URL})
	if (*got)[0].method != "GET" || (*got)[0].path != "/api/v1/databases/m/mongo/collections" {
		t.Errorf("request = %+v", (*got)[0])
	}
	if strings.ContainsRune(stdout, 0x1b) || !strings.Contains(stdout, "COLLECTION") || !strings.Contains(stdout, "users") {
		t.Errorf("stdout = %q", stdout)
	}
	js, _ := runCLIExpectOK(t, []string{"databases", "mongo", "collections", "m", "--json", "--api-url", srv.URL})
	if !json.Valid([]byte(js)) || !strings.Contains(js, `"fingerprint"`) {
		t.Errorf("json = %q", js)
	}
}

func TestRun_DatabasesMongoFindSendsFlagsAndServerGuardDecides(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&m)
		b, _ := json.Marshal(m)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(m["filter"]), "$where") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"filter operator not allowed"}`))
			return
		}
		_, _ = w.Write([]byte(`{"documents":[{"_id":1,"n":"a"}],"truncated":true,"next_after":1,"fingerprint":"f","duration_ms":2}`))
	}))
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"databases", "mongo", "find", "m", "--database", "app", "--collection", "users",
		"--filter", `{"n":"a"}`, "--sort", `{"_id":1}`, "--limit", "5", "--after", "7", "--api-url", srv.URL})
	for _, w := range []string{`"collection":"users"`, `"filter":{"n":"a"}`, `"limit":5`, `"after":7`} {
		if !strings.Contains(body, w) {
			t.Errorf("body %s missing %s", body, w)
		}
	}
	if !strings.Contains(stdout, `{"_id":1,"n":"a"}`) || !strings.Contains(stdout, "--after '1'") {
		t.Errorf("stdout = %q", stdout)
	}

	// The client does not pre-judge operators; the server's refusal is surfaced.
	stderr, code := runCLICode([]string{"databases", "mongo", "find", "m", "--database", "app", "--collection", "users",
		"--filter", `{"$where":"1"}`, "--api-url", srv.URL})
	if code == 0 || !strings.Contains(stderr, "filter operator not allowed") {
		t.Errorf("code = %d stderr = %q", code, stderr)
	}
	if !strings.Contains(body, "$where") {
		t.Errorf("the filter must reach the server unchanged: %s", body)
	}
}

func TestRun_DatabasesMongoFindNeedsTarget(t *testing.T) {
	srv, got := newRecordingServer(t, `{}`)
	_, code := runCLICode([]string{"databases", "mongo", "find", "m", "--api-url", srv.URL})
	if code == 0 || len(*got) != 0 {
		t.Errorf("code = %d requests = %+v", code, *got)
	}
}

func runCLICode(args []string) (stderr string, code int) {
	var o, e bytes.Buffer
	code = run("levelrail-cli-test", args, &o, &e, envMap())
	return e.String(), code
}
