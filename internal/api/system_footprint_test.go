package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

func TestIdleFootprintEndpoint(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/footprint", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("no probe = %d, want 501", rec.Code)
	}

	t.Setenv(footprint.EnvTolRSSPct, "1000")
	t.Setenv(footprint.EnvTolGoroutines, "100000")
	t.Setenv(footprint.EnvTolFDs, "100000")
	t.Setenv(footprint.EnvTolCPUPoints, "1000")
	rt.SetControlPlaneProbe(ControlPlaneProbe{DB: &fakeCPDB{}, Reconcile: func() reconcile.Stats { return reconcile.Stats{} }, AppCount: func(context.Context) int { return 3 }})

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/footprint", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}
	var a footprint.Assessment
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.Current.Apps != 3 || a.Current.Goroutines == 0 || a.Platform == "" || a.Checks == nil {
		t.Fatalf("assessment = %+v", a)
	}
	switch a.Status {
	case footprint.StatusWithin:
		if len(a.Checks) != 4 || a.Baseline == nil || a.Tolerance == nil {
			t.Fatalf("within without checks: %+v", a)
		}
	case footprint.StatusNoBaseline:
		if a.Baseline != nil {
			t.Fatalf("no_baseline carries a row: %+v", a)
		}
	default:
		t.Fatalf("status = %s with huge tolerances", a.Status)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/control-plane-health", ""))
	if !strings.Contains(rec.Body.String(), `"idle_footprint"`) {
		t.Fatalf("health body lacks idle_footprint: %s", rec.Body.String())
	}
}
