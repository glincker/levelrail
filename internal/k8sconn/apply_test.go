package k8sconn

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

type patchRec struct {
	Path, Query, ContentType string
}

// applyFake keeps applied objects by path, like a tiny API server.
type applyFake struct {
	mu      sync.Mutex
	objects map[string]string
	patches []patchRec
}

func newApplyFake(t *testing.T, seed map[string]string) (*Client, *applyFake) {
	t.Helper()
	f := &applyFake{objects: seed}
	if f.objects == nil {
		f.objects = map[string]string{}
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			if b, ok := f.objects[r.URL.Path]; ok {
				_, _ = w.Write([]byte(b))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			f.patches = append(f.patches, patchRec{r.URL.Path, r.URL.RawQuery, r.Header.Get("Content-Type")})
			f.objects[r.URL.Path] = string(b)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), Config{Creds: serverCreds(srv), Guard: trafficpolicy.EgressGuard{AllowLoopback: true}, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestApplyServerSide(t *testing.T) {
	c, f := newApplyFake(t, nil)
	spec := baseSpec()
	r, err := Render(spec)
	if err != nil {
		t.Fatal(err)
	}
	names, err := c.Apply(context.Background(), spec, r)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names, ","); got != "Deployment/whoami,Service/whoami,Ingress/whoami" {
		t.Fatalf("names = %s", got)
	}
	if len(f.patches) != 4 {
		t.Fatalf("patches = %d, want namespace plus three objects", len(f.patches))
	}
	for _, p := range f.patches {
		if p.ContentType != "application/apply-patch+yaml" || !strings.Contains(p.Query, "fieldManager=levelrail") || strings.Contains(p.Query, "force") {
			t.Fatalf("patch %+v: want apply content type, field manager, no force on new objects", p)
		}
	}
	ns := f.objects["/api/v1/namespaces/apps"]
	if !strings.Contains(ns, LabelManagedBy) {
		t.Fatalf("namespace not labelled: %s", ns)
	}

	// A second apply finds our labelled objects and may force.
	f.patches = nil
	if _, err := c.Apply(context.Background(), spec, r); err != nil {
		t.Fatal(err)
	}
	for _, p := range f.patches {
		if !strings.Contains(p.Query, "force=true") {
			t.Fatalf("re-apply of our own object should force: %+v", p)
		}
	}
	if !json.Valid([]byte(f.objects["/apis/apps/v1/namespaces/apps/deployments/whoami"])) {
		t.Fatal("applied body is not valid json")
	}
}

func TestApplyRefusesUnlabelledTakeover(t *testing.T) {
	c, f := newApplyFake(t, map[string]string{
		"/api/v1/namespaces/apps":                 `{"metadata":{"name":"apps"}}`,
		"/api/v1/namespaces/apps/services/whoami": `{"metadata":{"name":"whoami","labels":{"team":"other"}}}`,
	})
	spec := baseSpec()
	r, _ := Render(spec)
	_, err := c.Apply(context.Background(), spec, r)
	if !errors.Is(err, ErrNotManaged) {
		t.Fatalf("err = %v, want ErrNotManaged", err)
	}
	if len(f.patches) != 0 {
		t.Fatalf("nothing may change before every object is checked, got %+v", f.patches)
	}
}

func TestDeploymentStatusReadsReason(t *testing.T) {
	lbl := `"labels":{"app.kubernetes.io/managed-by":"levelrail","levelrail.dev/app":"whoami"}`
	c := newFake(t, map[string]string{
		"GET /apis/apps/v1/namespaces/apps/deployments/whoami":            `{"metadata":{` + lbl + `},"status":{"replicas":2,"readyReplicas":0}}`,
		"GET /api/v1/namespaces/apps/pods":                                `{"items":[{"metadata":{"name":"p1"},"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ImagePullBackOff"}}}]}}]}`,
		"GET /apis/networking.k8s.io/v1/namespaces/apps/ingresses/whoami": `{"status":{"loadBalancer":{"ingress":[{"ip":"10.0.0.9"}]}}}`,
	})
	st, err := c.DeploymentStatus(context.Background(), "apps", "whoami")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Found || st.ReadyReplicas != 0 || st.IngressAddress != "10.0.0.9" || !strings.Contains(st.Reason, "image could not be pulled") {
		t.Fatalf("status = %+v", st)
	}
	foreign := newFake(t, map[string]string{
		"GET /apis/apps/v1/namespaces/apps/deployments/whoami": `{"metadata":{"labels":{"x":"y"}}}`,
	})
	if st, _ := foreign.DeploymentStatus(context.Background(), "apps", "whoami"); st.Found {
		t.Fatal("an unlabelled object must read as missing")
	}
}
