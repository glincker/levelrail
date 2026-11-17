package k8sconn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const (
	ownLbl   = `"labels":{"app.kubernetes.io/managed-by":"levelrail","levelrail.dev/app":"web","levelrail.dev/instance":"i1"}`
	otherLbl = `"labels":{"team":"other"}`
)

type recKube struct {
	mu      sync.Mutex
	objects map[string]string
	deletes []string
	queries []string
}

func newRecKube(t *testing.T, objects map[string]string) (*Client, *recKube) {
	t.Helper()
	k := &recKube{objects: objects}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		defer k.mu.Unlock()
		if r.Method == http.MethodDelete {
			k.deletes = append(k.deletes, r.URL.Path)
			delete(k.objects, r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/log") {
			k.queries = append(k.queries, r.URL.RawQuery)
		}
		if b, ok := k.objects[r.URL.Path]; ok {
			_, _ = w.Write([]byte(b))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), Config{Creds: serverCreds(srv), Guard: trafficpolicy.EgressGuard{AllowLoopback: true}, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c, k
}

const ns = "/namespaces/apps"

func TestRemoveDeletesOnlyOurLabelledObjects(t *testing.T) {
	c, k := newRecKube(t, map[string]string{
		"/apis/apps/v1" + ns + "/deployments/web":            `{"metadata":{` + ownLbl + `}}`,
		"/api/v1" + ns + "/services/web":                     `{"metadata":{` + ownLbl + `}}`,
		"/api/v1" + ns + "/services/other":                   `{"metadata":{` + otherLbl + `}}`,
		"/apis/networking.k8s.io/v1" + ns + "/ingresses/web": `{"metadata":{` + ownLbl + `}}`,
	})
	res, err := c.Remove(context.Background(), "apps", "web", "i1")
	if err != nil || len(res.Deleted) != 3 {
		t.Fatalf("remove = %+v %v", res, err)
	}
	for _, d := range k.deletes {
		if strings.HasSuffix(d, "/apps") || strings.Contains(d, "other") {
			t.Fatalf("deleted something that is not ours: %s", d)
		}
	}
	// A second call finds nothing and still succeeds.
	res, err = c.Remove(context.Background(), "apps", "web", "i1")
	if err != nil || len(res.Deleted) != 0 || len(res.Absent) != 3 {
		t.Fatalf("idempotent remove = %+v %v", res, err)
	}
}

func TestRemoveRefusesUnlabelledAndDeletesNothing(t *testing.T) {
	c, k := newRecKube(t, map[string]string{
		"/apis/apps/v1" + ns + "/deployments/web": `{"metadata":{` + ownLbl + `}}`,
		"/api/v1" + ns + "/services/web":          `{"metadata":{` + otherLbl + `}}`,
	})
	_, err := c.Remove(context.Background(), "apps", "web", "i1")
	if !errors.Is(err, ErrNotManaged) || len(k.deletes) != 0 {
		t.Fatalf("err=%v deletes=%v", err, k.deletes)
	}
	// Another install's instance id is refused too.
	if _, err := c.Remove(context.Background(), "apps", "web", "i2"); !errors.Is(err, ErrNotManaged) || len(k.deletes) != 0 {
		t.Fatalf("foreign instance: err=%v deletes=%v", err, k.deletes)
	}
}

func TestPodLogsBounded(t *testing.T) {
	c, k := newRecKube(t, map[string]string{
		"/apis/apps/v1" + ns + "/deployments/web": `{"metadata":{` + ownLbl + `}}`,
		"/api/v1" + ns + "/pods":                  `{"items":[{"metadata":{"name":"web-1"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Running"}}]}`,
		"/api/v1" + ns + "/pods/web-1/log":        strings.Repeat("x", 100),
	})
	res, err := c.PodLogs(context.Background(), "apps", "web", "i1", "", "", 50, 10)
	if err != nil || len(res.Text) != 10 || !res.Truncated || res.Pod != "web-1" || res.Container != "app" {
		t.Fatalf("logs = %+v %v", res, err)
	}
	if !strings.Contains(k.queries[0], "tailLines=50") {
		t.Fatalf("query = %s", k.queries[0])
	}
	if _, err := c.PodLogs(context.Background(), "apps", "web", "i1", "kube-dns-1", "", 5, 10); !errors.Is(err, ErrLogTarget) {
		t.Fatalf("foreign pod = %v", err)
	}
	if _, err := c.PodLogs(context.Background(), "apps", "web", "i1", "", "sidecar", 5, 10); !errors.Is(err, ErrLogTarget) {
		t.Fatalf("unknown container = %v", err)
	}
}
