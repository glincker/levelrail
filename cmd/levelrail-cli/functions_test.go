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

func TestRun_FunctionsDeployListInvokeDelete(t *testing.T) {
	var fnSrv *httptest.Server
	var created appResource
	var sleepBody, invokeBody, invokeMethod string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			_ = json.NewDecoder(r.Body).Decode(&created)
			created.FallbackURL = fnSrv.URL
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/fn/sleep":
			b, _ := io.ReadAll(r.Body)
			sleepBody = string(b)
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "idle_minutes": 10, "hold_requests": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps":
			_ = json.NewEncoder(w).Encode([]appResource{{Name: "fn", Image: "x:1", FallbackURL: fnSrv.URL}, {Name: "plain", Image: "y:1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/fn/sleep":
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "idle_minutes": 10, "hold_requests": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/plain/sleep":
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": false})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/fn":
			_ = json.NewEncoder(w).Encode(appResource{Name: "fn", FallbackURL: fnSrv.URL})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/fn":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)
	fnSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/done", http.StatusTemporaryRedirect)
			return
		}
		b, _ := io.ReadAll(r.Body)
		invokeMethod, invokeBody = r.Method, string(b)
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(fnSrv.Close)

	call := func(args ...string) (string, int) {
		var stdout, stderr bytes.Buffer
		args = append(args, "--token", "t", "--api-url", api.URL)
		code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		return stdout.String(), code
	}

	out, code := call("functions", "deploy", "fn", "--image", "x:1", "--port", "9000", "--idle", "15m", "--env", "A=1")
	if code != exitOK || !strings.Contains(out, "deployed") {
		t.Fatalf("deploy = %d %q", code, out)
	}
	if created.Name != "fn" || created.Port != 9000 || created.Env["A"] != "1" {
		t.Errorf("created app = %+v", created)
	}
	if sleepBody != `{"hold_requests":true,"idle_minutes":15}` {
		t.Errorf("sleep body = %s, want idle 15 with hold on", sleepBody)
	}

	if out, code := call("functions", "list"); code != exitOK || !strings.Contains(out, "fn") || strings.Contains(out, "plain") {
		t.Fatalf("list = %d %q, want only the function", code, out)
	}

	if out, code := call("functions", "invoke", "fn", "--path", "/start", "--method", "POST", "--data", "payload"); code != exitOK || out != "hello" {
		t.Fatalf("invoke = %d %q", code, out)
	}
	if invokeMethod != http.MethodPost || invokeBody != "payload" {
		t.Errorf("a 307 must replay the POST with its body, got %s %q", invokeMethod, invokeBody)
	}

	if _, code := call("functions", "delete", "fn"); code != exitOK {
		t.Fatalf("delete = %d", code)
	}
}
