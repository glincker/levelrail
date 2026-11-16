package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"encoding/pem"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

type deployKube struct {
	mu      sync.Mutex
	objects map[string]string
	patches map[string]string
}

func startDeployKube(t *testing.T, seed map[string]string) (*httptest.Server, *deployKube) {
	t.Helper()
	k := &deployKube{objects: seed, patches: map[string]string{}}
	if k.objects == nil {
		k.objects = map[string]string{}
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		defer k.mu.Unlock()
		if r.Method == http.MethodPatch {
			b, _ := io.ReadAll(r.Body)
			if r.Header.Get("Content-Type") != "application/apply-patch+yaml" || !strings.Contains(r.URL.RawQuery, "fieldManager=levelrail") {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			k.patches[r.URL.Path] = string(b)
			k.objects[r.URL.Path] = string(b)
			_, _ = w.Write(b)
			return
		}
		if b, ok := k.objects[r.URL.Path]; ok {
			_, _ = w.Write([]byte(b))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv, k
}

func newDeployHarness(t *testing.T, kube *httptest.Server) func(method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	caB64 := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: kube.Certificate().Raw}))
	db := openTestDB(t)
	sec := &fakeK8sSecrets{vals: map[string]string{}}
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithNodeProviderSecrets(sec))
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) {
		if k == "APP_K8S_CLUSTERS_ENABLED" || k == "APP_K8S_ALLOW_PRIVATE_API" {
			return "true", true
		}
		return "", false
	})
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	raw, _ := json.Marshal(map[string]string{"name": "dev", "kubeconfig": k8sTestConfig(kube.URL, caB64, "    token: tkn-deploy-1")})
	if rec := do("POST", k8sClustersPath, string(raw)); rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	return do
}

const deployBody = `{"app":"whoami","image":"traefik/whoami:v1.10","port":80,"replicas":2,"host":"whoami.example.com","env":{"SECRET_THING":"hunter2"}}`

func TestK8sDeployApplyAndStatus(t *testing.T) {
	lbl := `"labels":{"app.kubernetes.io/managed-by":"levelrail","levelrail.dev/app":"whoami"}`
	kube, k := startDeployKube(t, map[string]string{
		"/apis/apps/v1/namespaces/levelrail-managed/deployments/whoami": `{"metadata":{` + lbl + `},"status":{"replicas":2,"readyReplicas":0}}`,
		"/api/v1/namespaces/levelrail-managed/pods":                     `{"items":[{"metadata":{"name":"p1"},"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ImagePullBackOff"}}}]}}]}`,
	})
	// The deployment object above is pre-existing and labelled, so the apply may force it.
	do := newDeployHarness(t, kube)

	rec := do("POST", k8sClustersPath+"/dev/deployments", deployBody)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "Deployment/whoami") || strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("deploy = %d %s", rec.Code, rec.Body.String())
	}
	dep := k.patches["/apis/apps/v1/namespaces/levelrail-managed/deployments/whoami"]
	if !strings.Contains(dep, `"runAsNonRoot":true`) || !strings.Contains(dep, `"runAsUser":10001`) || !strings.Contains(dep, `"limits"`) {
		t.Fatalf("default deploy must be non-root with limits: %s", dep)
	}
	if !strings.Contains(k.patches["/api/v1/namespaces/levelrail-managed"], "managed-by") {
		t.Fatal("default namespace was not created with the label")
	}
	if rec := do("GET", k8sClustersPath+"/dev/deployments", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"app":"whoami"`) || strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	rec = do("GET", k8sClustersPath+"/dev/deployments/whoami", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "image could not be pulled") || !strings.Contains(rec.Body.String(), `"ready":false`) {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do("GET", k8sClustersPath+"/dev/deployments/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown deployment = %d", rec.Code)
	}
}

func TestK8sDeployRootPolicy(t *testing.T) {
	kube, k := startDeployKube(t, nil)
	do := newDeployHarness(t, kube)
	if rec := do("POST", k8sClustersPath+"/dev/deployments", `{"app":"a","image":"nginx:1","port":80,"run_as_user":0}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("root without allow_root = %d, want 400", rec.Code)
	}
	if len(k.patches) != 0 {
		t.Fatal("a refused request must not reach the cluster")
	}
	if rec := do("POST", k8sClustersPath+"/dev/deployments", `{"app":"a","image":"nginx:1","port":80,"allow_root":true}`); rec.Code != http.StatusCreated {
		t.Fatalf("allow_root = %d %s", rec.Code, rec.Body.String())
	}
	if dep := k.patches["/apis/apps/v1/namespaces/levelrail-managed/deployments/a"]; strings.Contains(dep, "runAsNonRoot") {
		t.Fatalf("allow_root should leave the image user alone: %s", dep)
	}
	for _, bad := range []string{
		`{"app":"a","image":"localhost:5000/x","port":80}`,
		`{"app":"a","image":"nginx:1","port":80,"replicas":999}`,
		`{"app":"A","image":"nginx:1","port":80}`,
	} {
		if rec := do("POST", k8sClustersPath+"/dev/deployments", bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, rec.Code)
		}
	}
}

func TestK8sDeployRefusesUnlabelledObject(t *testing.T) {
	kube, k := startDeployKube(t, map[string]string{
		"/api/v1/namespaces/levelrail-managed":                 `{"metadata":{"name":"levelrail-managed"}}`,
		"/api/v1/namespaces/levelrail-managed/services/whoami": `{"metadata":{"labels":{"team":"other"}}}`,
	})
	do := newDeployHarness(t, kube)
	rec := do("POST", k8sClustersPath+"/dev/deployments", deployBody)
	if rec.Code != http.StatusConflict || len(k.patches) != 0 {
		t.Fatalf("takeover = %d patches=%d %s", rec.Code, len(k.patches), rec.Body.String())
	}
}

func TestK8sDeployMissingCluster(t *testing.T) {
	kube, _ := startDeployKube(t, nil)
	do := newDeployHarness(t, kube)
	if rec := do("POST", k8sClustersPath+"/ghost/deployments", deployBody); rec.Code != http.StatusNotFound {
		t.Fatalf("missing cluster = %d, want 404", rec.Code)
	}
	if rec := do("GET", k8sClustersPath+"/ghost/deployments/x", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing cluster status = %d, want 404", rec.Code)
	}
}
