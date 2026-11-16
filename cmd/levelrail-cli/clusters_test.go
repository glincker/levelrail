package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_ClustersConnectSendsKubeconfigFromFile(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"lab","server":"https://10.0.0.1:6443","auth_mode":"token"}`))
	}))
	defer srv.Close()

	kc := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(kc, []byte("apiVersion: v1\nSECRET-MARKER"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := runCLIExpectOK(t, []string{"clusters", "connect", "lab", "--kubeconfig", kc, "--context", "c1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/clusters" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotBody["name"] != "lab" || gotBody["context"] != "c1" || !strings.Contains(gotBody["kubeconfig"], "SECRET-MARKER") {
		t.Errorf("body = %v", gotBody)
	}
	if strings.Contains(stdout+stderr, "SECRET-MARKER") {
		t.Error("kubeconfig content was echoed")
	}
}

func TestRun_ClustersNodesAndFlagErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/clusters/lab/nodes" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"nodes":[{"name":"n1","ready":true,"roles":["control-plane"],"kubelet_version":"v1.30"}]}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"clusters", "nodes", "lab", "--api-url", srv.URL})
	if !strings.Contains(stdout, "n1") {
		t.Errorf("stdout = %q", stdout)
	}
	// Both sources, or neither, is a usage error before any request.
	if got := run("t", []string{"clusters", "connect", "lab", "--api-url", srv.URL}, io.Discard, io.Discard, envMap()); got == exitOK {
		t.Error("connect without a credential source should fail")
	}
}

func TestClustersRBACYAMLIsNamespaceScoped(t *testing.T) {
	out, _ := runCLIExpectOK(t, []string{"clusters", "rbac", "--namespace", "apps"})
	if strings.Contains(out, "ClusterRole") || strings.Count(out, "namespace: apps") != 4 {
		t.Errorf("yaml not namespace scoped:\n%s", out)
	}
	if got := run("t", []string{"clusters", "rbac", "--namespace", "Bad_NS"}, io.Discard, io.Discard, envMap()); got != exitUsage {
		t.Errorf("exit = %d, want usage", got)
	}
}
