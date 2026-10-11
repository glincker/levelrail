package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordedReq struct {
	method, path, query, body string
}

func composeFakeServer(t *testing.T, reqs *[]recordedReq) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, recordedReq{r.Method, r.URL.Path, r.URL.RawQuery, string(b)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/apps/stack/compose" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"app_id":"stack","phase":"degraded","revision":2,"services":[{"key":"db","name":"stack-db","image":"postgres:16","ready":"True","replicas":1},{"key":"web","name":"stack-web","image":"nginx","ready":"False","reason":"CreateFailed","replicas":1}]}`)
		case r.URL.Path == "/api/v1/apps/stack/compose" && r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"app_id":"stack","revision":3,"services":[{"name":"stack-web","image":"nginx"}],"skipped_services":["debug"]}`)
		case r.URL.Path == "/api/v1/apps/stack/compose" && r.Method == http.MethodDelete:
			_, _ = io.WriteString(w, `{"app_id":"stack","removed":["stack-web"],"volumes_kept":["app-stack-db-data"]}`)
		case strings.HasSuffix(r.URL.Path, "/rollback"):
			_, _ = io.WriteString(w, `{"app_id":"stack","rolled_back_to":1,"deploy":{"app_id":"stack","revision":4}}`)
		case strings.HasSuffix(r.URL.Path, "/redeploy"):
			_, _ = io.WriteString(w, `{"app_id":"stack","restarted":["stack-web"]}`)
		case strings.HasSuffix(r.URL.Path, "/revisions"):
			_, _ = io.WriteString(w, `{"revisions":[{"revision":2,"source":"api","created_at":"2026-10-10T00:00:00Z"}]}`)
		case r.URL.Path == "/api/v1/compose/preview":
			_, _ = io.WriteString(w, `{"valid":false,"errors":["service \"a\": privileged: true is not allowed"],"services":[{"key":"a","image":"x"}]}`)
		case strings.HasSuffix(r.URL.Path, "/install"):
			_, _ = io.WriteString(w, `{"app_id":"wiki","revision":1,"services":[{"name":"wiki-wiki","image":"wiki:2"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found"}`)
		}
	}))
}

func TestRun_Compose(t *testing.T) {
	file := writeComposeFixture(t)
	tests := []struct {
		name      string
		args      []string
		wantExit  int
		wantReq   recordedReq
		wantOut   string
		checkBody func(t *testing.T, body string)
	}{
		{name: "deploy with profile", args: []string{"compose", "deploy", "stack", "-f", file, "--profile-name", "debug"}, wantReq: recordedReq{method: "POST", path: "/api/v1/apps/stack/compose", query: "profiles=debug"}, wantOut: "skipped (profile not enabled): debug",
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "nginx:latest") {
					t.Errorf("body = %q, want the compose file", body)
				}
			}},
		{name: "status", args: []string{"compose", "status", "stack"}, wantReq: recordedReq{method: "GET", path: "/api/v1/apps/stack/compose"}, wantOut: "CreateFailed"},
		{name: "revisions", args: []string{"compose", "revisions", "stack"}, wantReq: recordedReq{method: "GET", path: "/api/v1/apps/stack/compose/revisions"}, wantOut: "2"},
		{name: "rollback to revision", args: []string{"compose", "rollback", "stack", "--revision", "1"}, wantReq: recordedReq{method: "POST", path: "/api/v1/apps/stack/compose/rollback"}, wantOut: "rolled back to revision 1",
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, `"revision":1`) {
					t.Errorf("body = %q", body)
				}
			}},
		{name: "redeploy one service", args: []string{"compose", "redeploy", "stack", "--service", "web"}, wantReq: recordedReq{method: "POST", path: "/api/v1/apps/stack/compose/redeploy"}, wantOut: "stack-web"},
		{name: "down keeps volumes", args: []string{"compose", "down", "stack"}, wantReq: recordedReq{method: "DELETE", path: "/api/v1/apps/stack/compose"}, wantOut: "volumes kept"},
		{name: "down with volumes", args: []string{"compose", "down", "stack", "--volumes"}, wantReq: recordedReq{method: "DELETE", path: "/api/v1/apps/stack/compose", query: "volumes=true"}},
		{name: "invalid preview exits non-zero", args: []string{"compose", "preview", "-f", file}, wantExit: exitValidation, wantReq: recordedReq{method: "POST", path: "/api/v1/compose/preview"}, wantOut: "privileged"},
		{name: "template install", args: []string{"templates", "install", "wikijs", "--name", "wiki", "--domain", "wiki=wiki.example.com"}, wantReq: recordedReq{method: "POST", path: "/api/v1/service-templates/wikijs/install"}, wantOut: "installed from wikijs",
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, `"domains":{"wiki":"wiki.example.com"}`) || !strings.Contains(body, `"name":"wiki"`) {
					t.Errorf("body = %q", body)
				}
			}},
		{name: "missing app name", args: []string{"compose", "status"}, wantExit: exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqs []recordedReq
			srv := composeFakeServer(t, &reqs)
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			got := run("lr", append(tt.args, "--api-url", srv.URL), &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantReq.path != "" {
				if len(reqs) != 1 {
					t.Fatalf("got %d requests, want 1: %+v", len(reqs), reqs)
				}
				r := reqs[0]
				if r.method != tt.wantReq.method || r.path != tt.wantReq.path || r.query != tt.wantReq.query {
					t.Errorf("request = %+v, want %+v", r, tt.wantReq)
				}
				if tt.checkBody != nil {
					tt.checkBody(t, r.body)
				}
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

func TestForwardServiceLogArgs(t *testing.T) {
	got := forwardServiceLogArgs([]string{"stack", "--service", "web", "--since", "5m", "--api-url", "http://x"}, "stack", "web")
	want := []string{"stack-web", "--since", "5m", "--api-url", "http://x"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("forwardServiceLogArgs() = %v, want %v", got, want)
	}
}
