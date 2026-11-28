package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func seedLogsRow(t *testing.T, db *store.DB) {
	t.Helper()
	row := store.K8sDeployment{Cluster: "dev", App: "whoami", Namespace: "levelrail-managed", Image: "traefik/whoami:v1", Port: 80, Replicas: 1}
	if err := db.UpsertK8sDeployment(context.Background(), row); err != nil {
		t.Fatal(err)
	}
}

const logsInstLbl = `"labels":{"app.kubernetes.io/managed-by":"levelrail","levelrail.dev/app":"whoami","levelrail.dev/instance":"` + testInstanceID + `"}`

type logsKube struct {
	mu      sync.Mutex
	deletes []string
	gone    map[string]bool
	tails   []string
}

func startLogsKube(t *testing.T, serviceLabels string) (*httptest.Server, *logsKube) {
	t.Helper()
	k := &logsKube{gone: map[string]bool{}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		defer k.mu.Unlock()
		p := r.URL.Path
		if r.Method == http.MethodPost && strings.HasSuffix(p, "/selfsubjectrulesreviews") {
			_, _ = w.Write([]byte(`{"status":{"resourceRules":[{"verbs":["get"],"apiGroups":[""],"resources":["pods"]}]}}`))
			return
		}
		if r.Method == http.MethodDelete {
			k.deletes = append(k.deletes, p)
			k.gone[p] = true
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if k.gone[p] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch p {
		case "/apis/apps/v1/namespaces/levelrail-managed/deployments/whoami", "/apis/networking.k8s.io/v1/namespaces/levelrail-managed/ingresses/whoami":
			_, _ = w.Write([]byte(`{"metadata":{` + logsInstLbl + `},"status":{}}`))
		case "/api/v1/namespaces/levelrail-managed/services/whoami":
			_, _ = w.Write([]byte(`{"metadata":{` + serviceLabels + `}}`))
		case "/api/v1/namespaces/levelrail-managed/pods":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"whoami-1"},"spec":{"containers":[{"name":"whoami"}]},"status":{"phase":"Running"}}]}`))
		case "/api/v1/namespaces/levelrail-managed/pods/whoami-1/log":
			k.tails = append(k.tails, r.URL.Query().Get("tailLines"))
			_, _ = w.Write([]byte("line one\nline two\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, k
}

func TestK8sLogsTailBoundedAndRemoveOurs(t *testing.T) {
	kube, k := startLogsKube(t, logsInstLbl)
	do, db := newDeployEnv(t, kube, map[string]string{"APP_K8S_LOG_TAIL_LINES_MAX": "20"})
	seedLogsRow(t, db)
	rec := do("GET", k8sClustersPath+"/dev/deployments/whoami/logs?tail=9999", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Body.String(), "line two") {
		t.Fatalf("logs = %d %s", rec.Code, rec.Body.String())
	}
	if len(k.tails) != 1 || k.tails[0] != "20" {
		t.Fatalf("tail was not capped: %v", k.tails)
	}
	if rec := do("GET", k8sClustersPath+"/dev/deployments/whoami/logs?pod=other-1", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign pod = %d", rec.Code)
	}
	if rec := do("GET", k8sClustersPath+"/dev/deployments/whoami/logs?tail=abc", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad tail = %d", rec.Code)
	}
	rec = do("DELETE", k8sClustersPath+"/dev/deployments/whoami", "")
	if rec.Code != http.StatusOK || len(k.deletes) != 3 {
		t.Fatalf("remove = %d deletes=%v %s", rec.Code, k.deletes, rec.Body.String())
	}
	for _, d := range k.deletes {
		if !strings.Contains(d, "/whoami") {
			t.Fatalf("unexpected delete %s", d)
		}
	}
	if rec := do("GET", k8sClustersPath+"/dev/deployments/whoami", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("row should be gone: %d", rec.Code)
	}
	if rec := do("DELETE", k8sClustersPath+"/dev/deployments/whoami", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second remove (row gone) = %d", rec.Code)
	}
	if rec := do("DELETE", k8sClustersPath+"/ghost/deployments/whoami", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing cluster = %d", rec.Code)
	}
	if rec := do("GET", k8sClustersPath+"/ghost/deployments/whoami/logs", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing cluster logs = %d", rec.Code)
	}
}

func TestK8sRemoveRefusesUnlabelledService(t *testing.T) {
	kube, k := startLogsKube(t, `"labels":{"team":"other"}`)
	do, db := newDeployEnv(t, kube, nil)
	seedLogsRow(t, db)
	if rec := do("DELETE", k8sClustersPath+"/dev/deployments/whoami", ""); rec.Code != http.StatusConflict {
		t.Fatalf("remove over foreign service = %d", rec.Code)
	}
	if len(k.deletes) != 0 {
		t.Fatalf("deletes = %v", k.deletes)
	}
}

func TestK8sRemoveOnlyReportsRecordedObjectsAsAbsent(t *testing.T) {
	kube, k := startLogsKube(t, logsInstLbl)
	k.gone["/apis/networking.k8s.io/v1/namespaces/levelrail-managed/ingresses/whoami"] = true
	do, db := newDeployEnv(t, kube, nil)
	row := store.K8sDeployment{Cluster: "dev", App: "whoami", Namespace: "levelrail-managed", Image: "traefik/whoami:v1", Port: 80, Replicas: 1,
		Objects: []string{"Deployment/whoami", "Service/whoami"}}
	if err := db.UpsertK8sDeployment(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	rec := do("DELETE", k8sClustersPath+"/dev/deployments/whoami", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Ingress/whoami") {
		t.Fatalf("remove = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRecordedOnly(t *testing.T) {
	got := recordedOnly([]string{"Ingress/a", "Secret/s"}, []string{"Deployment/a", "Secret/s"})
	if len(got) != 1 || got[0] != "Secret/s" {
		t.Errorf("recordedOnly = %v", got)
	}
	if got := recordedOnly([]string{"Ingress/a"}, nil); len(got) != 1 {
		t.Errorf("no record keeps the list, got %v", got)
	}
}
