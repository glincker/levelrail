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
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestK8sConnectRefusesClusterAdminUnlessOverriddenAndSettingOn(t *testing.T) {
	kube := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"resourceRules":[{"verbs":["*"],"apiGroups":["*"],"resources":["*"]}]}}`))
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
	connect := func(name string, allow bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{
			"name": name, "kubeconfig": k8sTestConfig(kube.URL, caB64, "    token: tok-1234"), "allow_cluster_admin": allow,
		})
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, "POST", k8sClustersPath, string(raw)))
		return rec
	}

	rec := connect("adm", false)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "clusters rbac") {
		t.Fatalf("admin connect = %d %s, want 409 pointing at clusters rbac", rec.Code, rec.Body.String())
	}
	if list, _ := db.ListK8sClusters(context.Background()); len(list) != 0 || len(sec.vals) != 0 {
		t.Fatalf("refused admin stored %d rows and %d secrets", len(list), len(sec.vals))
	}
	rec = connect("adm", true)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "k8s.allow_cluster_admin") {
		t.Fatalf("override without setting = %d %s, want 409", rec.Code, rec.Body.String())
	}

	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyK8sAllowClusterAdmin, json.RawMessage(`true`), "test"); err != nil {
		t.Fatal(err)
	}
	if rec := connect("adm", false); rec.Code != http.StatusConflict {
		t.Fatalf("setting on but no request flag = %d, want 409", rec.Code)
	}
	if rec := connect("adm", true); rec.Code != http.StatusCreated {
		t.Fatalf("override plus setting = %d %s, want 201", rec.Code, rec.Body.String())
	}

	entries, _ := db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{})
	var refused, overridden bool
	for _, e := range entries {
		refused = refused || (e.Action == auditK8sClusterConnectFailed && e.StatusCode == http.StatusConflict)
		overridden = overridden || e.Action == auditK8sClusterAdminOverride
	}
	if !refused || !overridden {
		t.Fatalf("audit missing refused=%v overridden=%v", refused, overridden)
	}
}

func TestK8sConnectFailsClosedWhenRulesReviewErrors(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"error":      func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"incomplete": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":{"incomplete":true}}`)) },
	} {
		t.Run(name, func(t *testing.T) {
			kube := httptest.NewTLSServer(handler)
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
			connect := func(allow bool) *httptest.ResponseRecorder {
				raw, _ := json.Marshal(map[string]any{
					"name": "unv", "kubeconfig": k8sTestConfig(kube.URL, caB64, "    token: tok-1234"), "allow_cluster_admin": allow,
				})
				rec := httptest.NewRecorder()
				rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, "POST", k8sClustersPath, string(raw)))
				return rec
			}
			rec := connect(false)
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "could not verify the credential is not cluster-admin") {
				t.Fatalf("unverified connect = %d %s, want 409", rec.Code, rec.Body.String())
			}
			if list, _ := db.ListK8sClusters(context.Background()); len(list) != 0 || len(sec.vals) != 0 {
				t.Fatalf("refused connect stored %d rows and %d secrets", len(list), len(sec.vals))
			}
			entries, _ := db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{})
			var refused bool
			for _, e := range entries {
				refused = refused || (e.Action == auditK8sClusterConnectFailed && e.StatusCode == http.StatusConflict)
			}
			if !refused {
				t.Fatal("refusal was not audited")
			}
			if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyK8sAllowClusterAdmin, json.RawMessage(`true`), "test"); err != nil {
				t.Fatal(err)
			}
			if rec := connect(true); rec.Code != http.StatusCreated {
				t.Fatalf("override plus setting = %d %s, want 201", rec.Code, rec.Body.String())
			}
		})
	}
}
