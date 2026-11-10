package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/clitree"
)

func inspectServer(t *testing.T, body string, gotPath, gotQuery *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath, *gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAppsInspect_PendingChangesJSONUnchanged(t *testing.T) {
	// Odd spacing, key order and an unknown field must survive byte for byte.
	body := `{"pending":true, "changes":[{"field":"env.PORT","from":"80","to":"8080"}],"zeta_unknown":{"x":1}}`
	var path, query string
	srv := inspectServer(t, body+"\n", &path, &query)

	var stdout, stderr strings.Builder
	code := run("levelrail-cli-test", []string{"apps", "inspect", "web", "pending-changes", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr %s", code, stderr.String())
	}
	if path != "/api/v1/apps/web/pending-changes" {
		t.Errorf("path = %q", path)
	}
	if stdout.String() != body+"\n" {
		t.Errorf("stdout = %q, want the body unchanged", stdout.String())
	}
}

func TestAppsInspect_EffectiveConfigKeyFilter(t *testing.T) {
	body := `{"context":"production","env":[{"key":"API_TOKEN","value":"********","secret":true,"source":"app"},{"key":"PORT","value":"8080","secret":false,"source":"app"}],"build_command":{"value":"npm run build","source":"app"},"publish_dir":{"value":"","source":""}}`
	var path, query string
	srv := inspectServer(t, body, &path, &query)

	var stdout, stderr strings.Builder
	code := run("levelrail-cli-test", []string{"apps", "inspect", "web", "effective-config", "--key", "PORT", "--context", "preview", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr %s", code, stderr.String())
	}
	if path != "/api/v1/apps/web/effective-config" || query != "context=preview" {
		t.Errorf("request = %s?%s", path, query)
	}
	var got struct {
		Context string `json:"context"`
		Env     []struct {
			Key string `json:"key"`
		} `json:"env"`
		BuildCommand struct {
			Value string `json:"value"`
		} `json:"build_command"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout.String(), err)
	}
	if len(got.Env) != 1 || got.Env[0].Key != "PORT" || got.Context != "production" || got.BuildCommand.Value != "npm run build" {
		t.Errorf("filtered = %+v", got)
	}

	stdout.Reset()
	stderr.Reset()
	code = run("levelrail-cli-test", []string{"apps", "inspect", "web", "effective-config", "--key", "API_TOKEN", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("table exit = %d, stderr %s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "API_TOKEN") || !strings.Contains(out, "********") || strings.Contains(out, "PORT") {
		t.Errorf("table = %q, want only the masked API_TOKEN row", out)
	}

	stderr.Reset()
	if code := run("levelrail-cli-test", []string{"apps", "inspect", "web", "effective-config", "--key", "MISSING", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitValidation {
		t.Errorf("missing key exit = %d, want %d", code, exitValidation)
	}
}

func TestAppsInspect_PlainOutputIsTerminalSafe(t *testing.T) {
	var path, query string
	srv := inspectServer(t, `{"tag_moved":true,"reason":"bad\u001b[31mred"}`, &path, &query)

	var stdout, stderr strings.Builder
	code := run("levelrail-cli-test", []string{"apps", "inspect", "image-status", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr %s", code, stderr.String())
	}
	if path != "/api/v1/apps/web/image-status" {
		t.Errorf("path = %q, want view-first order accepted", path)
	}
	if out := stdout.String(); strings.ContainsRune(out, 0x1b) || !strings.Contains(out, "tag_moved: true") {
		t.Errorf("stdout = %q", out)
	}
	if code := run("levelrail-cli-test", []string{"apps", "inspect", "web", "rollback-targets", "--key", "X", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitValidation {
		t.Errorf("--key on another view exit = %d, want %d", code, exitValidation)
	}
}

func TestAppsInspectIsInTheCompletionTree(t *testing.T) {
	node := clitree.Tree["apps"].Subs["inspect"]
	if node == nil {
		t.Fatal("apps inspect missing from the completion tree")
	}
	for _, v := range appsInspectViews {
		if _, ok := node.Subs[v]; !ok {
			t.Errorf("apps inspect %s missing", v)
		}
	}
}
