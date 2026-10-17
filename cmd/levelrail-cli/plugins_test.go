package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

type pluginCall struct {
	method, path string
	body         map[string]any
}

func pluginServer(t *testing.T, calls *[]pluginCall) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := pluginCall{method: r.Method, path: r.URL.EscapedPath()}
		_ = json.NewDecoder(r.Body).Decode(&c.body)
		*calls = append(*calls, c)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/plugins" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]apiclient.Plugin{{Name: "example", Version: "1.0.0", Kind: "notification_channel", State: "installed", RequestedAbilities: []string{"read"}, ApprovedAbilities: []string{}}})
		case r.URL.Path == "/api/v1/plugin-keys" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]apiclient.PluginKey{})
		case r.URL.Path == "/api/v1/plugin-keys":
			_ = json.NewEncoder(w).Encode(apiclient.PluginKey{KeyID: "ed25519:ab", Name: "pub"})
		default:
			_ = json.NewEncoder(w).Encode(apiclient.Plugin{Name: "example", Version: "1.0.0", Kind: "notification_channel", State: "installed", RequestedAbilities: []string{"read"}, ApprovedAbilities: []string{}})
		}
	}))
}

func TestRun_Plugins_Lifecycle(t *testing.T) {
	experimental.Set(experimental.Plugins)
	restoreAllExperimental(t)
	var calls []pluginCall
	srv := pluginServer(t, &calls)
	defer srv.Close()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "plugin.yaml")
	sig := filepath.Join(dir, "plugin.sig")
	if err := os.WriteFile(manifest, []byte("api_version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sig, []byte("c2ln\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := []string{"--api-url", srv.URL, "--token", "t"}
	run := func(args ...string) string {
		out, _ := runCLIExpectOK(t, append(args, api...))
		return out
	}

	if out := run("plugins", "list"); !strings.Contains(out, "example") || !strings.Contains(out, "installed") {
		t.Errorf("list = %q", out)
	}
	if out := run("plugins", "show", "example"); !strings.Contains(out, "Requested:  read") || !strings.Contains(out, "Approved:   none") {
		t.Errorf("show = %q", out)
	}
	out := run("plugins", "install", "--manifest", manifest, "--signature", sig, "--key-id", "ed25519:ab", "--digest", "sha256:x")
	if !strings.Contains(out, "not approved or running") || !strings.Contains(out, "plugins approve example --abilities read") {
		t.Errorf("install output must point at the approval step: %q", out)
	}
	install := calls[len(calls)-1]
	if install.body["manifest"] != "api_version: 1\n" || install.body["signature"] != "c2ln" || install.body["expected_digest"] != "sha256:x" {
		t.Errorf("install body = %v (the manifest must travel byte for byte, the signature trimmed)", install.body)
	}
	run("plugins", "approve", "example", "--abilities", "read")
	run("plugins", "enable", "example")
	run("plugins", "disable", "example")
	run("plugins", "remove", "example")
	run("plugins", "trust", "add", "--name", "pub", "--public-key", "AAAA")
	if out := run("plugins", "trust", "list"); !strings.Contains(out, "no third-party plugin can be installed") {
		t.Errorf("empty trust list output = %q", out)
	}
	run("plugins", "trust", "remove", "ed25519:ab")

	want := []string{
		"GET /api/v1/plugins", "GET /api/v1/plugins/example", "POST /api/v1/plugins", "POST /api/v1/plugins/example/approve",
		"POST /api/v1/plugins/example/enable", "POST /api/v1/plugins/example/disable", "DELETE /api/v1/plugins/example",
		"POST /api/v1/plugin-keys", "GET /api/v1/plugin-keys", "DELETE /api/v1/plugin-keys/ed25519:ab",
	}
	for i, w := range want {
		if got := calls[i].method + " " + calls[i].path; got != w {
			t.Errorf("call %d = %q, want %q", i, got, w)
		}
	}
}

func TestRun_Plugins_ApproveNeedsExplicitAbilities(t *testing.T) {
	experimental.Set(experimental.Plugins)
	restoreAllExperimental(t)
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"plugins", "approve", "example"}, &stdout, &stderr, envMap()); code != exitUsage {
		t.Fatalf("exit = %d, want usage", code)
	}
	if !strings.Contains(stderr.String(), "nothing is approved by default") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRun_Plugins_HiddenWhenFeatureOff(t *testing.T) {
	experimental.Set()
	restoreAllExperimental(t)
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"plugins", "list"}, &stdout, &stderr, envMap()); code != exitUsage {
		t.Fatalf("exit = %d, want usage", code)
	}
	if !strings.Contains(stderr.String(), "APP_EXPERIMENTAL") {
		t.Errorf("stderr = %q", stderr.String())
	}
}
