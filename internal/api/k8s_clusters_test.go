package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

type fakeK8sSecrets struct {
	mu   sync.Mutex
	vals map[string]string
}

func (f *fakeK8sSecrets) SetValue(_ context.Context, svc, key, v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vals[svc+"|"+key] = v
	return nil
}

func (f *fakeK8sSecrets) Exists(_ context.Context, svc, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.vals[svc+"|"+key]
	return ok, nil
}

func (f *fakeK8sSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vals[svc+"|"+key], nil
}

func (f *fakeK8sSecrets) DeleteAll(_ context.Context, svc string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.vals {
		if strings.HasPrefix(k, svc+"|") {
			delete(f.vals, k)
		}
	}
	return nil
}

func k8sTestConfig(server, caB64, user string) string {
	return "apiVersion: v1\nkind: Config\ncurrent-context: c\n" +
		"clusters:\n- name: cl\n  cluster:\n    server: " + server + "\n    certificate-authority-data: " + caB64 + "\n" +
		"users:\n- name: u\n  user:\n" + user + "\n" +
		"contexts:\n- name: c\n  context:\n    cluster: cl\n    user: u\n"
}

func TestK8sConnectStoresSecretsAndNeverReturnsThem(t *testing.T) {
	kube := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"resourceRules":[{"verbs":["get"],"apiGroups":[""],"resources":["pods"]}]}}`))
	}))
	defer kube.Close()
	caB64 := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: kube.Certificate().Raw}))

	db := openTestDB(t)
	sec := &fakeK8sSecrets{vals: map[string]string{}}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithNodeProviderSecrets(sec))
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
	connect := func(name, user string) *httptest.ResponseRecorder {
		cfg := k8sTestConfig(kube.URL, caB64, user)
		raw, _ := json.Marshal(map[string]string{"name": name, "kubeconfig": cfg})
		body := string(raw)
		return do("POST", k8sClustersPath, body)
	}

	if rec := connect("bad", "    exec:\n      command: aws"); rec.Code != http.StatusBadRequest {
		t.Fatalf("exec kubeconfig = %d %s, want 400", rec.Code, rec.Body.String())
	}
	if list, _ := db.ListK8sClusters(context.Background()); len(list) != 0 || len(sec.vals) != 0 {
		t.Fatalf("refused kubeconfig stored %d rows and %d secrets", len(list), len(sec.vals))
	}

	const token = "sa-token-value-123" //nolint:gosec // fixture
	rec := connect("dev", "    token: "+token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	if sec.vals["k8s-cluster/dev|token"] != token {
		t.Fatalf("token was not stored in the secrets store")
	}
	for _, path := range []string{k8sClustersPath, k8sClustersPath + "/dev"} {
		got := do("GET", path, "").Body.String()
		if strings.Contains(got, token) || strings.Contains(got, "BEGIN CERTIFICATE") || !strings.Contains(got, `"ca_fingerprint"`) {
			t.Fatalf("GET %s leaks or lacks fields: %s", path, got)
		}
	}
	if rec := do("POST", k8sClustersPath+"/dev/test", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("test = %d %s", rec.Code, rec.Body.String())
	}
	if rec := connect("dev", "    token: "+token); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", rec.Code)
	}
	if rec := do("DELETE", k8sClustersPath+"/dev", ""); rec.Code != http.StatusNoContent || len(sec.vals) != 0 {
		t.Fatalf("delete = %d, secrets left %d", rec.Code, len(sec.vals))
	}
	if _, err := db.GetK8sCluster(context.Background(), "dev"); err == nil {
		t.Fatal("row survived delete")
	}
}

func TestK8sRoutesOffByDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, "GET", k8sClustersPath, ""))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "k8s.clusters_enabled") {
		t.Fatalf("GET = %d %s, want 403 naming the setting", rec.Code, rec.Body.String())
	}
}
