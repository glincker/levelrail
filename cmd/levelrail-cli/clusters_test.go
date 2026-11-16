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

func TestRun_ClustersDeploySendsEnvFromFileAndHidesValues(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"app":"web","namespace":"apps","objects":["Deployment/web"]}`))
	}))
	defer srv.Close()
	ef := filepath.Join(t.TempDir(), "app.env")
	if err := os.WriteFile(ef, []byte("DB_PASS=SECRET-MARKER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := runCLIExpectOK(t, []string{"clusters", "deploy", "lab", "--app", "web", "--image", "nginx:1", "--port", "80", "--env-file", ef, "--replicas", "2", "--api-url", srv.URL})
	env, _ := gotBody["env"].(map[string]any)
	if gotPath != "/api/v1/clusters/lab/deployments" || env["DB_PASS"] != "SECRET-MARKER" || gotBody["replicas"] != float64(2) {
		t.Errorf("path=%s body=%v", gotPath, gotBody)
	}
	if strings.Contains(stdout+stderr, "SECRET-MARKER") || !strings.Contains(stdout, "Deployment/web") {
		t.Errorf("output = %q", stdout)
	}
}

func TestRun_ClustersStatusPrintsReadiness(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/clusters/lab/deployments/web" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"app":"web","namespace":"apps","found":true,"replicas":2,"ready_replicas":1,"conditions":[{"type":"Available","status":"False","reason":"MinimumReplicasUnavailable"}],"ingress_address":"10.0.0.5","reason":"rolling"}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"clusters", "status", "lab", "web", "--api-url", srv.URL})
	for _, want := range []string{"1/2 replicas ready", "ingress: 10.0.0.5", "MinimumReplicasUnavailable", "reason: rolling"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in %q", want, stdout)
		}
	}
}

func TestClustersRBACClusterReadFlagAddsSeparateReadOnlyRole(t *testing.T) {
	out, _ := runCLIExpectOK(t, []string{"clusters", "rbac", "--namespace", "apps", "--cluster-read"})
	for _, want := range []string{"kind: ClusterRole\n", "kind: ClusterRoleBinding", `resources: ["nodes", "namespaces"]`, `verbs: ["get", "list"]`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "delete") && strings.Contains(out[strings.Index(out, "kind: ClusterRole\n"):], "delete") {
		t.Errorf("cluster role must be read only:\n%s", out)
	}
}

func TestRun_ClustersLogsAndRemove(t *testing.T) {
	var gotPath, gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		if strings.HasSuffix(r.URL.Path, "/logs") {
			_, _ = w.Write([]byte("hello\n"))
			return
		}
		_, _ = w.Write([]byte(`{"app":"web","namespace":"ns","deleted":["Deployment/web"],"absent":[]}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"clusters", "logs", "lab", "web", "--tail", "5", "--pod", "web-1", "--api-url", srv.URL})
	if gotPath != "/api/v1/clusters/lab/deployments/web/logs" || !strings.Contains(gotQuery, "tail=5") || !strings.Contains(gotQuery, "pod=web-1") || !strings.Contains(stdout, "hello") {
		t.Errorf("logs: path=%s query=%s out=%q", gotPath, gotQuery, stdout)
	}
	gotMethod = ""
	stdout, _ = runCLIExpectOK(t, []string{"clusters", "remove", "lab", "web", "--api-url", srv.URL})
	if gotMethod != "" || !strings.Contains(stdout, "preview only") {
		t.Errorf("remove without --yes must not call the API: %q %q", gotMethod, stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"clusters", "remove", "lab", "web", "--yes", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/clusters/lab/deployments/web" || !strings.Contains(stdout, "deleted Deployment/web") {
		t.Errorf("remove: %s %s %q", gotMethod, gotPath, stdout)
	}
	if got := run("t", []string{"clusters", "remove", "lab", "--yes", "--api-url", srv.URL}, io.Discard, io.Discard, envMap()); got == exitOK {
		t.Error("remove without an app should fail")
	}
}
