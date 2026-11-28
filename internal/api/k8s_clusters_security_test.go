package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/secrets"
	"github.com/glincker/levelrail/internal/store"
)

// chaosK8sSecrets shares one namespace map like the real store and can fail on demand.
type chaosK8sSecrets struct {
	*fakeK8sSecrets
	failDeleteAll bool
	resolveErr    error
}

func (c *chaosK8sSecrets) DeleteAll(ctx context.Context, svc string) error {
	if c.failDeleteAll {
		return errors.New("delete failed")
	}
	return c.fakeK8sSecrets.DeleteAll(ctx, svc)
}

func (c *chaosK8sSecrets) Resolve(ctx context.Context, svc, key string) (string, error) {
	if c.resolveErr != nil {
		return "", c.resolveErr
	}
	v, _ := c.fakeK8sSecrets.Resolve(ctx, svc, key)
	if v == "" {
		return "", secrets.ErrValueNotFound
	}
	return v, nil
}

type k8sEnv struct {
	t      *testing.T
	rt     *Router
	db     *store.DB
	sec    *chaosK8sSecrets
	cookie *http.Cookie
	kube   *httptest.Server
	caB64  string
}

func newK8sEnv(t *testing.T, allowPrivate, withSecrets bool) *k8sEnv {
	t.Helper()
	kube := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"resourceRules":[{"verbs":["get"],"apiGroups":[""],"resources":["pods"]}]}}`))
	}))
	t.Cleanup(kube.Close)
	db := openTestDB(t)
	e := &k8sEnv{t: t, db: db, kube: kube, sec: &chaosK8sSecrets{fakeK8sSecrets: &fakeK8sSecrets{vals: map[string]string{}}},
		caB64: base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: kube.Certificate().Raw}))}
	e.rt = e.router(t, true, allowPrivate, withSecrets)
	e.cookie = loginTestSession(t, e.rt, db)
	return e
}

func (e *k8sEnv) router(t *testing.T, enabled, allowPrivate, withSecrets bool) *Router {
	t.Helper()
	var opts []Option
	if withSecrets {
		opts = append(opts, WithNodeProviderSecrets(e.sec))
	}
	rt := NewRouter(discardLogger(), testBrand(), e.db, opts...)
	rt.runtimeSettings = runtimesettings.New(e.db, func(k string) (string, bool) {
		if (k == "APP_K8S_CLUSTERS_ENABLED" && enabled) || (k == "APP_K8S_ALLOW_PRIVATE_API" && allowPrivate) {
			return "true", true
		}
		return "", false
	})
	return rt
}

func (e *k8sEnv) do(rt *Router, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(e.t, e.cookie, method, path, body))
	return rec
}

func (e *k8sEnv) connect(rt *Router, server, token string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(map[string]string{"name": "dev", "server": server, "token": token, "ca": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: e.kube.Certificate().Raw}))})
	return e.do(rt, "POST", k8sClustersPath, string(raw))
}

func TestK8sConnectWithoutMasterKeyIs501(t *testing.T) {
	e := newK8sEnv(t, true, false)
	if rec := e.connect(e.rt, e.kube.URL, "tok"); rec.Code != http.StatusNotImplemented {
		t.Fatalf("connect = %d, want 501", rec.Code)
	}
}

func TestK8sConnectRefusedWhilePrivateOffNamesSetting(t *testing.T) {
	e := newK8sEnv(t, false, true)
	rec := e.connect(e.rt, e.kube.URL, "tok")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "k8s.allow_private_api") || strings.Contains(rec.Body.String(), "FORWARDER") {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	if len(e.sec.vals) != 0 {
		t.Fatal("refused connect stored secrets")
	}
	entries, _ := e.db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{})
	found := false
	for _, a := range entries {
		if a.Action == auditK8sClusterConnectFailed && a.StatusCode == http.StatusBadRequest {
			found = true
		}
	}
	if !found {
		t.Fatal("refused connect was not audited")
	}
}

func TestK8sConnectRejectsUserinfoServer(t *testing.T) {
	e := newK8sEnv(t, true, true)
	rec := e.connect(e.rt, strings.Replace(e.kube.URL, "https://", "https://bob:hunter2@", 1), "tok")
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	entries, _ := e.db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{})
	for _, a := range entries {
		if strings.Contains(a.Detail, "hunter2") {
			t.Fatal("credentials reached the audit log")
		}
	}
}

func TestK8sReconnectNeverReusesStaleSecrets(t *testing.T) {
	e := newK8sEnv(t, true, true)
	e.sec.vals["k8s-cluster/dev|client_key"] = "stale-key"
	e.sec.vals["k8s-cluster/dev|token"] = "stale-token"
	if rec := e.connect(e.rt, e.kube.URL, "fresh"); rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := e.sec.vals["k8s-cluster/dev|client_key"]; ok || e.sec.vals["k8s-cluster/dev|token"] != "fresh" {
		t.Fatalf("stale secrets survived: %v", e.sec.vals)
	}
}

func TestK8sConflictDeletesNothing(t *testing.T) {
	e := newK8sEnv(t, true, true)
	if rec := e.connect(e.rt, e.kube.URL, "first"); rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d", rec.Code)
	}
	if rec := e.connect(e.rt, e.kube.URL, "second"); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", rec.Code)
	}
	if e.sec.vals["k8s-cluster/dev|token"] != "first" {
		t.Fatalf("conflict touched the winner's secrets: %v", e.sec.vals)
	}
}

func TestK8sConcurrentDuplicateConnectHasOneWinner(t *testing.T) {
	e := newK8sEnv(t, true, true)
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.connect(e.rt, e.kube.URL, "tok").Code
		}()
	}
	wg.Wait()
	created := 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
		default:
			t.Fatalf("unexpected status %d in %v", c, codes)
		}
	}
	if created != 1 || e.sec.vals["k8s-cluster/dev|token"] != "tok" {
		t.Fatalf("created = %d, secrets = %v", created, e.sec.vals)
	}
}

func TestK8sConnectDeleteAllFailureRollsBackRow(t *testing.T) {
	e := newK8sEnv(t, true, true)
	e.sec.failDeleteAll = true
	if rec := e.connect(e.rt, e.kube.URL, "tok"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("connect = %d, want 500", rec.Code)
	}
	if _, err := e.db.GetK8sCluster(context.Background(), "dev"); err == nil {
		t.Fatal("row survived a failed secret clear")
	}
}

func TestK8sDisconnectFailsWhenSecretDeleteFails(t *testing.T) {
	e := newK8sEnv(t, true, true)
	if rec := e.connect(e.rt, e.kube.URL, "tok"); rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d", rec.Code)
	}
	e.sec.failDeleteAll = true
	if rec := e.do(e.rt, "DELETE", k8sClustersPath+"/dev", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("delete = %d, want 500", rec.Code)
	}
	if _, err := e.db.GetK8sCluster(context.Background(), "dev"); err != nil {
		t.Fatal("row was removed while secrets remain")
	}
}

func TestK8sDisconnectWorksWhileFeatureOff(t *testing.T) {
	e := newK8sEnv(t, true, true)
	if rec := e.connect(e.rt, e.kube.URL, "tok"); rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d", rec.Code)
	}
	off := e.router(t, false, true, true)
	if rec := e.do(off, "GET", k8sClustersPath, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("list while off = %d, want 403", rec.Code)
	}
	if rec := e.do(off, "DELETE", k8sClustersPath+"/dev", ""); rec.Code != http.StatusNoContent || len(e.sec.vals) != 0 {
		t.Fatalf("delete while off = %d, secrets left %d", rec.Code, len(e.sec.vals))
	}
}

func TestK8sLoadFailsClosed(t *testing.T) {
	e := newK8sEnv(t, true, true)
	if rec := e.connect(e.rt, e.kube.URL, "tok"); rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d", rec.Code)
	}
	row, err := e.db.GetK8sCluster(context.Background(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.rt.loadK8sClient(context.Background(), row); err != nil {
		t.Fatalf("healthy load failed: %v", err)
	}
	e.sec.resolveErr = errors.New("db locked")
	if _, err := e.rt.loadK8sClient(context.Background(), row); err == nil {
		t.Fatal("resolve error did not fail the load")
	}
	if rec := e.do(e.rt, "POST", k8sClustersPath+"/dev/test", ""); !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("test route = %s, want ok false", rec.Body.String())
	}
	e.sec.resolveErr = nil
	delete(e.sec.vals, "k8s-cluster/dev|ca")
	if _, err := e.rt.loadK8sClient(context.Background(), row); err == nil {
		t.Fatal("missing ca with a stored fingerprint did not fail the load")
	}
}
