package k8sconn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestApplyRefusesUnmanagedNamespace(t *testing.T) {
	c, f := newApplyFake(t, map[string]string{"/api/v1/namespaces/apps": `{"metadata":{"name":"apps"}}`})
	spec := baseSpec()
	r, _ := Render(spec)
	if _, err := c.Apply(context.Background(), spec, r); !errors.Is(err, ErrNamespaceNotManaged) {
		t.Fatalf("err = %v, want ErrNamespaceNotManaged", err)
	}
	spec.AllowUnmanagedNamespace = true
	if _, err := c.Apply(context.Background(), spec, r); err != nil {
		t.Fatalf("allowed namespace: %v", err)
	}
	if len(f.posts) != 3 {
		t.Fatalf("posts = %v, want the three objects", f.posts)
	}
}

func TestApplyRefusesHostOfForeignIngress(t *testing.T) {
	c, f := newApplyFake(t, map[string]string{
		"/apis/networking.k8s.io/v1/ingresses": `{"items":[{"metadata":{"name":"shop","namespace":"prod"},"spec":{"rules":[{"host":"Whoami.Example.com"}]}}]}`,
	})
	spec := baseSpec()
	r, _ := Render(spec)
	if _, err := c.Apply(context.Background(), spec, r); !errors.Is(err, ErrHostTaken) {
		t.Fatalf("err = %v, want ErrHostTaken", err)
	}
	if len(f.posts)+len(f.patches) != 0 {
		t.Fatal("nothing may change when the host is taken")
	}
}

func TestApplyCreateConflictIsNeverMerged(t *testing.T) {
	// GET says missing, POST says it exists now: a foreign object appeared in between.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch:
			t.Error("a patch must never be sent for an object that was missing")
		case r.Method == http.MethodPost && r.URL.Path != "/api/v1/namespaces":
			w.WriteHeader(http.StatusConflict)
		case r.Method == http.MethodGet && r.URL.Path == "/apis/networking.k8s.io/v1/ingresses":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/apps":
			_, _ = w.Write([]byte(`{"metadata":{"labels":{"app.kubernetes.io/managed-by":"levelrail"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), Config{Creds: serverCreds(srv), Guard: trafficpolicy.EgressGuard{AllowLoopback: true}, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	spec := baseSpec()
	r, _ := Render(spec)
	if _, err := c.Apply(context.Background(), spec, r); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("err = %v, want ErrNotManaged", err)
	}
}

func TestOwnedByChecksInstance(t *testing.T) {
	l := Labels("whoami", "mine")
	if !ownedBy(l, "whoami", "mine") || ownedBy(l, "whoami", "other") || ownedBy(Labels("whoami", ""), "whoami", "mine") {
		t.Fatal("instance label must match when an instance id is set")
	}
	r, err := Render(AppSpec{Name: "whoami", Namespace: "apps", InstanceID: "mine", Image: "x:1", Port: 80, Replicas: 1,
		Resources: baseSpec().Resources})
	if err != nil {
		t.Fatal(err)
	}
	sel, _ := r.Service["spec"].(map[string]any)["selector"].(map[string]any)
	if sel[LabelInstance] != "mine" {
		t.Fatalf("service selector %v lacks the instance label", sel)
	}
}

func TestRenderRejectsLeadingDigitName(t *testing.T) {
	s := baseSpec()
	s.Name = "1app"
	if _, err := Render(s); err == nil {
		t.Fatal("a name starting with a digit must be refused")
	}
}
