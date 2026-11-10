package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func viewsServer(t *testing.T, got *map[string]any, deleted *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(got)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sv_1","name":"failing","list":"apps","filter":{"status":"failing"},"shared":"team","mine":true}`))
		case http.MethodDelete:
			*deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{"views":[{"id":"sv_1","name":"failing\u001b[2J","list":"apps","filter":{"status":"failing","q":"api web"},"shared":"team","mine":true},` +
				`{"id":"sv_2","name":"pg","list":"databases","filter":{"engine":"postgres"},"shared":"team","mine":false}]}`))
		}
	}))
}

func TestRun_ViewsSaveSendsFilterAndShare(t *testing.T) {
	var got map[string]any
	var deleted string
	srv := viewsServer(t, &got, &deleted)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"views", "save", "failing", "--list", "apps", "--filter", "status=failing", "--filter", "q=a=b", "--share", "team", "--api-url", srv.URL})
	f, _ := got["filter"].(map[string]any)
	if got["name"] != "failing" || got["list"] != "apps" || got["shared"] != "team" || f["status"] != "failing" || f["q"] != "a=b" {
		t.Fatalf("body %v", got)
	}
	if !strings.Contains(stdout, "sv_1") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestRun_ViewsListApplyDelete(t *testing.T) {
	var got map[string]any
	var deleted string
	srv := viewsServer(t, &got, &deleted)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"views", "list", "--api-url", srv.URL})
	if strings.Contains(stdout, "\x1b") || !strings.Contains(stdout, "pg") {
		t.Fatalf("list %q", stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"views", "apply", "sv_1", "--api-url", srv.URL})
	if strings.TrimSpace(stdout) != "/apps?q=api+web&status=failing" {
		t.Fatalf("apply %q", stdout)
	}
	runCLIExpectOK(t, []string{"views", "delete", "sv_2", "--api-url", srv.URL})
	if deleted != "/api/v1/saved-views/sv_2" {
		t.Fatalf("deleted %q", deleted)
	}
}

func TestRun_ViewsRejectsBadFilter(t *testing.T) {
	code := run("levelrail-cli", []string{"views", "save", "x", "--list", "apps", "--filter", "novalue", "--api-url", "http://127.0.0.1:1"}, &strings.Builder{}, &strings.Builder{}, func(string) (string, bool) { return "", false })
	if code == exitOK {
		t.Fatal("bad filter accepted")
	}
}
