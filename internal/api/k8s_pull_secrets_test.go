package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const pullPassword = "p@ss-pull-9f3a" //nolint:gosec // fixture

// pullKube is a small stateful cluster: create 409s on a duplicate, delete 404s when absent.
type pullKube struct {
	mu      sync.Mutex
	objects map[string]string
	deletes []string
}

func startPullKube(t *testing.T, seed map[string]string) (*httptest.Server, *pullKube) {
	t.Helper()
	k := &pullKube{objects: map[string]string{}}
	for p, b := range seed {
		k.objects[p] = b
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		defer k.mu.Unlock()
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/selfsubjectrulesreviews"):
			_, _ = w.Write([]byte(`{"status":{"resourceRules":[{"verbs":["get"],"apiGroups":[""],"resources":["pods"]}]}}`))
		case r.Method == http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			name := pullBetween(string(b), `"name":"`, `"`)
			if _, ok := k.objects[p+"/"+name]; ok {
				w.WriteHeader(http.StatusConflict)
				return
			}
			k.objects[p+"/"+name] = string(b)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(b)
		case r.Method == http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			k.objects[p] = string(b)
			_, _ = w.Write(b)
		case r.Method == http.MethodDelete:
			if _, ok := k.objects[p]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(k.objects, p)
			k.deletes = append(k.deletes, p)
			_, _ = w.Write([]byte(`{}`))
		default:
			if b, ok := k.objects[p]; ok {
				_, _ = w.Write([]byte(b))
				return
			}
			if p == "/apis/networking.k8s.io/v1/ingresses" {
				_, _ = w.Write([]byte(`{"items":[]}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, k
}

func pullBetween(s, a, b string) string {
	_, rest, _ := strings.Cut(s, a)
	v, _, _ := strings.Cut(rest, b)
	return v
}

const pullAddBody = `{"name":"ghcr-main","registry":"https://GHCR.io/","username":"bot","password":"` + pullPassword + `"}`

func TestK8sPullSecretDeployCreatesAndRemovesSecret(t *testing.T) {
	kube, k := startPullKube(t, nil)
	do, db := newDeployEnv(t, kube, nil)

	rec := do("POST", k8sClustersPath+"/dev/pull-secrets", pullAddBody)
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), pullPassword) || !strings.Contains(rec.Body.String(), `"registry":"ghcr.io"`) {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	rec = do("GET", k8sClustersPath+"/dev/pull-secrets", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ghcr-main") || strings.Contains(rec.Body.String(), pullPassword) || strings.Contains(rec.Body.String(), "bot") {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	rec = do("POST", k8sClustersPath+"/dev/deployments", `{"app":"api","image":"ghcr.io/acme/api:1","port":80,"pull_secret":"ghcr-main"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "Secret/api-registry") || strings.Contains(rec.Body.String(), pullPassword) {
		t.Fatalf("deploy = %d %s", rec.Code, rec.Body.String())
	}
	sec := k.objects["/api/v1/namespaces/levelrail-managed/secrets/api-registry"]
	auth := base64.StdEncoding.EncodeToString([]byte("bot:" + pullPassword))
	cfg, _ := base64.StdEncoding.DecodeString(pullBetween(sec, `"data":{".dockerconfigjson":"`, `"`))
	for _, want := range []string{`"type":"kubernetes.io/dockerconfigjson"`, `"levelrail.dev/app":"api"`, `"levelrail.dev/instance":"` + testInstanceID + `"`, `"app.kubernetes.io/managed-by":"levelrail"`} {
		if !strings.Contains(sec, want) {
			t.Fatalf("secret lacks %s: %s", want, sec)
		}
	}
	if !strings.Contains(string(cfg), auth) || !strings.Contains(string(cfg), `"ghcr.io"`) {
		t.Fatalf("dockerconfigjson wrong: %s", cfg)
	}
	if dep := k.objects["/apis/apps/v1/namespaces/levelrail-managed/deployments/api"]; !strings.Contains(dep, `"imagePullSecrets":[{"name":"api-registry"}]`) {
		t.Fatalf("deployment does not reference the secret: %s", dep)
	}

	var details string
	rows, err := db.Query(`SELECT detail FROM audit_log`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var d string
		_ = rows.Scan(&d)
		details += d
	}
	_ = rows.Close()
	if strings.Contains(details, pullPassword) || strings.Contains(details, auth) || !strings.Contains(details, "ghcr-main") {
		t.Fatalf("audit leaks the credential or misses the name: %s", details)
	}

	k.objects["/api/v1/namespaces/levelrail-managed/secrets/other"] = `{"metadata":{"labels":{"team":"x"}}}`
	rec = do("DELETE", k8sClustersPath+"/dev/deployments/api", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Secret/api-registry") {
		t.Fatalf("remove = %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := k.objects["/api/v1/namespaces/levelrail-managed/secrets/api-registry"]; ok {
		t.Fatal("remove left the pull secret behind")
	}
	if _, ok := k.objects["/api/v1/namespaces/levelrail-managed/secrets/other"]; !ok {
		t.Fatal("remove deleted a secret it did not create")
	}

	if rec := do("DELETE", k8sClustersPath+"/dev/pull-secrets/ghcr-main", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete pull secret = %d", rec.Code)
	}
	if rec := do("DELETE", k8sClustersPath+"/dev/pull-secrets/ghcr-main", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
}

func TestK8sPullSecretRegistryMismatchRefused(t *testing.T) {
	kube, k := startPullKube(t, nil)
	do, _ := newDeployEnv(t, kube, nil)
	do("POST", k8sClustersPath+"/dev/pull-secrets", pullAddBody)
	for _, img := range []string{"registry.example.com/acme/api:1", "nginx:1", "ghcr.io.evil.com/a:1"} {
		rec := do("POST", k8sClustersPath+"/dev/deployments", `{"app":"api","image":"`+img+`","port":80,"pull_secret":"ghcr-main"}`)
		if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), pullPassword) {
			t.Fatalf("%s = %d %s, want 400", img, rec.Code, rec.Body.String())
		}
	}
	if rec := do("POST", k8sClustersPath+"/dev/deployments", `{"app":"api","image":"ghcr.io/a/b:1","port":80,"pull_secret":"missing"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown pull secret = %d", rec.Code)
	}
	if len(k.objects) != 0 {
		t.Fatalf("a refused deploy reached the cluster: %v", k.objects)
	}
}

func TestK8sPullSecretRedeployWithoutDeletesOwnSecret(t *testing.T) {
	kube, k := startPullKube(t, nil)
	do, _ := newDeployEnv(t, kube, nil)
	do("POST", k8sClustersPath+"/dev/pull-secrets", pullAddBody)
	do("POST", k8sClustersPath+"/dev/deployments", `{"app":"api","image":"ghcr.io/acme/api:1","port":80,"pull_secret":"ghcr-main"}`)
	if rec := do("POST", k8sClustersPath+"/dev/deployments", `{"app":"api","image":"nginx:1","port":80}`); rec.Code != http.StatusCreated {
		t.Fatalf("redeploy = %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := k.objects["/api/v1/namespaces/levelrail-managed/secrets/api-registry"]; ok {
		t.Fatal("stale pull secret was kept")
	}
}

func TestK8sPullSecretRefusesForeignSecretName(t *testing.T) {
	kube, k := startPullKube(t, map[string]string{ //nolint:gosec // fixture paths, not credentials
		"/api/v1/namespaces/levelrail-managed":                      `{"metadata":{"name":"levelrail-managed"}}`,
		"/api/v1/namespaces/levelrail-managed/secrets/api-registry": `{"metadata":{"labels":{"team":"x"}}}`,
	})
	do, _ := newDeployEnv(t, kube, nil)
	do("POST", k8sClustersPath+"/dev/pull-secrets", pullAddBody)
	rec := do("POST", k8sClustersPath+"/dev/deployments", `{"app":"api","image":"ghcr.io/acme/api:1","port":80,"pull_secret":"ghcr-main"}`)
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), pullPassword) {
		t.Fatalf("foreign secret = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(k.objects["/api/v1/namespaces/levelrail-managed/secrets/api-registry"], "dockerconfigjson") {
		t.Fatal("a foreign secret was overwritten")
	}
}

func TestK8sPullSecretStatusNamesSecretOnPullFailure(t *testing.T) {
	kube, k := startPullKube(t, nil)
	do, _ := newDeployEnv(t, kube, nil)
	do("POST", k8sClustersPath+"/dev/pull-secrets", pullAddBody)
	do("POST", k8sClustersPath+"/dev/deployments", `{"app":"api","image":"ghcr.io/acme/api:1","port":80,"pull_secret":"ghcr-main"}`)
	k.mu.Lock()
	k.objects["/api/v1/namespaces/levelrail-managed/pods"] = `{"items":[{"metadata":{"name":"p1"},"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ErrImagePull"}}}]}}]}`
	k.mu.Unlock()
	rec := do("GET", k8sClustersPath+"/dev/deployments/api", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "pull secret api-registry") {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}
