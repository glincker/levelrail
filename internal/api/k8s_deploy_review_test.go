package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/glincker/levelrail/internal/store"
)

func hasK8sAudit(t *testing.T, db *store.DB, action string, status int, contains string) bool {
	t.Helper()
	entries, _ := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	for _, a := range entries {
		if a.Action == action && a.StatusCode == status && strings.Contains(a.Detail, contains) {
			return true
		}
	}
	return false
}

func TestK8sDeployRefusesForeignNamespace(t *testing.T) {
	kube, k := startDeployKube(t, map[string]string{"/api/v1/namespaces/kube-system": `{"metadata":{"name":"kube-system"}}`})
	do, db := newDeployEnv(t, kube, nil)
	body := `{"app":"a","namespace":"kube-system","image":"nginx:1","port":80}`
	if rec := do("POST", k8sClustersPath+"/dev/deployments", body); rec.Code != http.StatusConflict || len(k.patches) != 0 {
		t.Fatalf("foreign namespace = %d patches=%d", rec.Code, len(k.patches))
	}
	if !hasK8sAudit(t, db, auditK8sDeployRefused, http.StatusConflict, `"namespace":"kube-system"`) {
		t.Fatal("namespace 409 was not audited with the effective namespace")
	}
	allowed, _ := newDeployEnv(t, kube, map[string]string{"APP_K8S_ALLOWED_NAMESPACES": "kube-system"})
	if rec := allowed("POST", k8sClustersPath+"/dev/deployments", body); rec.Code != http.StatusCreated {
		t.Fatalf("allow-listed namespace = %d %s", rec.Code, rec.Body.String())
	}
}

func TestK8sDeployRefusesHostOwnedByOtherIngress(t *testing.T) {
	kube, k := startDeployKube(t, map[string]string{
		"/apis/networking.k8s.io/v1/ingresses": `{"items":[{"metadata":{"name":"shop","namespace":"prod"},"spec":{"rules":[{"host":"whoami.example.com"}]}}]}`,
	})
	do, _ := newDeployEnv(t, kube, nil)
	if rec := do("POST", k8sClustersPath+"/dev/deployments", deployBody); rec.Code != http.StatusConflict || len(k.patches) != 0 {
		t.Fatalf("taken host = %d patches=%d %s", rec.Code, len(k.patches), rec.Body.String())
	}
}

func TestK8sDeployHostListForbidden(t *testing.T) {
	kube, _ := startDeployKube(t, map[string]string{"/apis/networking.k8s.io/v1/ingresses": "forbidden"})
	do, _ := newDeployEnv(t, kube, nil)
	rec := do("POST", k8sClustersPath+"/dev/deployments", deployBody)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "list ingresses across namespaces") || !strings.Contains(rec.Body.String(), "--cluster-read") {
		t.Fatalf("forbidden list = %d %s, want 409 naming the permission", rec.Code, rec.Body.String())
	}
	skip := strings.Replace(deployBody, `"host"`, `"skip_host_check":true,"host"`, 1)
	if rec := do("POST", k8sClustersPath+"/dev/deployments", skip); rec.Code != http.StatusBadRequest {
		t.Fatalf("override without setting = %d, want 400", rec.Code)
	}
	on, _ := newDeployEnv(t, kube, map[string]string{"APP_K8S_ALLOW_UNVERIFIED_HOSTS": "true"})
	if rec := on("POST", k8sClustersPath+"/dev/deployments", skip); rec.Code != http.StatusCreated {
		t.Fatalf("override with setting = %d %s", rec.Code, rec.Body.String())
	}
}

func TestK8sDeployConcurrentSameApp(t *testing.T) {
	kube, _ := startDeployKube(t, nil)
	do, _ := newDeployEnv(t, kube, nil)
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = do("POST", k8sClustersPath+"/dev/deployments", deployBody).Code
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
			t.Fatalf("codes = %v, want only 201 or 409", codes)
		}
	}
	if created == 0 {
		t.Fatalf("no deploy succeeded: %v", codes)
	}
}

func TestK8sDeployRejectsLeadingDigitName(t *testing.T) {
	kube, _ := startDeployKube(t, nil)
	do, _ := newDeployEnv(t, kube, nil)
	if rec := do("POST", k8sClustersPath+"/dev/deployments", `{"app":"1app","image":"nginx:1","port":80}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("leading digit = %d, want 400", rec.Code)
	}
}

func TestK8sDeployAuditsUpsertFailure(t *testing.T) {
	kube, _ := startDeployKube(t, nil)
	do, db := newDeployEnv(t, kube, nil)
	if _, err := db.ExecContext(context.Background(), `DROP TABLE k8s_deployments`); err != nil {
		t.Fatal(err)
	}
	if rec := do("POST", k8sClustersPath+"/dev/deployments", deployBody); rec.Code != http.StatusInternalServerError {
		t.Fatalf("upsert failure = %d, want 500", rec.Code)
	}
	if !hasK8sAudit(t, db, auditK8sDeployApplied, http.StatusCreated, `"replicas":2`) {
		t.Fatal("the applied entry must exist right after Apply")
	}
	if !hasK8sAudit(t, db, auditK8sDeployFailed, http.StatusInternalServerError, "cluster changed") {
		t.Fatal("upsert failure after apply was not audited")
	}
}

func TestK8sDisconnectLeavesNoStaleRows(t *testing.T) {
	kube, _ := startDeployKube(t, nil)
	do, db := newDeployEnv(t, kube, nil)
	if rec := do("POST", k8sClustersPath+"/dev/deployments", deployBody); rec.Code != http.StatusCreated {
		t.Fatalf("deploy = %d", rec.Code)
	}
	if rec := do("DELETE", k8sClustersPath+"/dev", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect = %d", rec.Code)
	}
	rows, err := db.ListK8sDeployments(context.Background(), "dev")
	if err != nil || len(rows) != 0 {
		t.Fatalf("stale rows after disconnect: %v %v", rows, err)
	}
}

func TestK8sPublicImageRegistryPort(t *testing.T) {
	for image, want := range map[string]bool{
		"nginx:1.27":                         true,
		"registry.example.com:5000/team/a:1": true,
		"localhost:5000/a:1":                 false,
		"a:b:c":                              false,
		"host:99999999/a":                    false,
	} {
		if publicImage(image) != want {
			t.Errorf("publicImage(%q) = %v, want %v", image, !want, want)
		}
	}
}

func TestTruncateK8sMessageKeepsUTF8(t *testing.T) {
	got := truncateK8sMessage(strings.Repeat("a", 299) + strings.Repeat("é", 5))
	if !utf8.ValidString(got) || len(got) > 300 {
		t.Fatalf("truncate produced %d bytes, valid=%v", len(got), utf8.ValidString(got))
	}
}
