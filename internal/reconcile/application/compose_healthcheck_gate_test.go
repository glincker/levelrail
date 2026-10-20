package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// Fake-runtime twin of the live compose healthcheck test: the compose probe, translated for real, gates the deploy.
func TestComposeHealthcheck_TranslatedProbeGatesDeploy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		path       string
		wantStatus reconcile.ConditionStatus
		wantReason string
	}{
		{"served path deploys", "/", reconcile.ConditionTrue, "Deployed"},
		{"unserved path fails readiness", "/this-path-never-exists", reconcile.ConditionFalse, "ReadinessFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "services:\n  web:\n    image: img:v1\n    ports:\n      - \"8080:8080\"\n    healthcheck:\n" +
				"      test: [\"CMD-SHELL\", \"curl -f http://localhost:8080" + tt.path + " || exit 1\"]\n" +
				"      interval: 1s\n      timeout: 1s\n      retries: 2\n"
			f, err := compose.Parse([]byte(yaml))
			if err != nil {
				t.Fatalf("compose.Parse() error = %v", err)
			}
			services, _, err := compose.ToDesiredServices("app", f)
			if err != nil || len(services) != 1 {
				t.Fatalf("ToDesiredServices() = %d services, err %v", len(services), err)
			}
			desired := services[0]
			if desired.Health == nil || desired.Health.Readiness == nil || desired.Health.Readiness.Path != tt.path {
				t.Fatalf("translated Health = %+v, want a readiness probe on %q", desired.Health, tt.path)
			}
			desired.Port = 80
			desired.Health.Readiness.Interval = 10 * time.Millisecond
			desired.Health.Readiness.Timeout = 100 * time.Millisecond

			rt := newFakeRuntime(serverPort(t, srv))
			c := New(desired.Name, &fakeStore{svc: &desired}, rt, WithReadyBudget(300*time.Millisecond))
			result, err := c.Reconcile(context.Background())
			if (err != nil) != (tt.wantStatus == reconcile.ConditionFalse) {
				t.Fatalf("Reconcile() error = %v, want failure=%v", err, tt.wantStatus == reconcile.ConditionFalse)
			}
			cond := conditionOf(t, result)
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Errorf("condition = %+v, want %s/%s", cond, tt.wantStatus, tt.wantReason)
			}
		})
	}
}
