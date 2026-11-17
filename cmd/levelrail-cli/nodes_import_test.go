package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesImport_MintsTokenForExistingInstance(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/resources") {
			_, _ = w.Write([]byte(`{"resources":[{"id":"us-east-1/i-0abc","name":"Web 1","region":"us-east-1","status":"running"}]}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		bb, _ := json.Marshal(req)
		gotBody = string(bb)
		_, _ = w.Write([]byte(`{"token":"tok-1"}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "import", "--provider", "aws", "--instance-id", "i-0abc", "--api-url", srv.URL})
	if !strings.Contains(stdout, "tok-1") || !strings.Contains(gotBody, `"node_name":"web-1"`) {
		t.Fatalf("stdout %q body %q", stdout, gotBody)
	}
}

func TestRun_NodesImport_SendsRegionQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/resources") {
			gotQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"resources":[{"id":"eu-west-1/i-0abc","name":"web","region":"eu-west-1","status":"running"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"token":"tok-2"}`))
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"nodes", "import", "--provider", "aws", "--region", "eu-west-1", "--instance-id", "i-0abc", "--api-url", srv.URL})
	if !strings.Contains(gotQuery, "region=eu-west-1") {
		t.Fatalf("query %q", gotQuery)
	}
}
