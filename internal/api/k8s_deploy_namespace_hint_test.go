package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestK8sDeployNamespaceCreateForbiddenNamesCauseAndFix(t *testing.T) {
	kube, k := startDeployKube(t, map[string]string{"forbid-post:/api/v1/namespaces": "1"})
	do, _ := newDeployEnv(t, kube, nil)
	rec := do("POST", k8sClustersPath+"/dev/deployments", deployBody)
	body := rec.Body.String()
	if rec.Code != http.StatusConflict || len(k.patches) != 0 {
		t.Fatalf("namespace create forbidden = %d patches=%d %s", rec.Code, len(k.patches), body)
	}
	for _, want := range []string{"cannot create namespaces", "clusters rbac --namespace", "k8s.allowed_namespaces"} {
		if !strings.Contains(body, want) {
			t.Errorf("message misses %q: %s", want, body)
		}
	}
}
