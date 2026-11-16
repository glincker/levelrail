package api

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"encoding/pem"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestK8sListNodesAndNamespaces(t *testing.T) {
	kube := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/nodes":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"n1","labels":{"node-role.kubernetes.io/control-plane":""}},"status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.30.0"}}}]}`))
		case "/api/v1/namespaces":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"default"},"status":{"phase":"Active"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer kube.Close()
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
	raw, _ := json.Marshal(map[string]string{"name": "dev", "kubeconfig": k8sTestConfig(kube.URL, caB64, "    token: tkn-read-1")})
	if rec := do("POST", k8sClustersPath, string(raw)); rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	rec := do("GET", k8sClustersPath+"/dev/nodes", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"n1"`) || !strings.Contains(rec.Body.String(), `"ready":true`) || !strings.Contains(rec.Body.String(), "control-plane") {
		t.Fatalf("nodes = %d %s", rec.Code, rec.Body.String())
	}
	rec = do("GET", k8sClustersPath+"/dev/namespaces", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"default"`) || !strings.Contains(rec.Body.String(), `"phase":"Active"`) {
		t.Fatalf("namespaces = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do("GET", k8sClustersPath+"/missing/nodes", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing cluster = %d, want 404", rec.Code)
	}
}
