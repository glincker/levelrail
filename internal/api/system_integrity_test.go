package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/integrity"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

type fakeIntegrity struct {
	status   integrity.Status
	restored string
	cancels  int
	checks   []string
	restore  error
}

func (f *fakeIntegrity) Status() integrity.Status { return f.status }
func (f *fakeIntegrity) View() (integrity.View, error) {
	return integrity.View{Status: f.status, Candidates: []integrity.Candidate{{Name: "snap.db", Verified: true}}}, nil
}
func (f *fakeIntegrity) Check(_ context.Context, mode string) integrity.Status {
	f.checks = append(f.checks, mode)
	return f.status
}
func (f *fakeIntegrity) Restore(_ context.Context, name, _ string) (integrity.Pending, error) {
	f.restored = name
	return integrity.Pending{Name: "snap.db"}, f.restore
}
func (f *fakeIntegrity) CancelRestore() error { f.cancels++; return nil }

type fakeGuard struct{ resets int }

func (f *fakeGuard) Status() reconcile.GuardStatus {
	return reconcile.GuardStatus{OpenBreakers: 1, Breakers: []reconcile.BreakerStatus{{Name: "app/a", State: reconcile.BreakerOpen}}}
}
func (f *fakeGuard) Reset(string) int { f.resets++; return 1 }

func TestIntegrityEndpointsAndFixes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fi := &fakeIntegrity{status: integrity.Status{Healthy: false, Error: "database file is corrupt: page 3"}}
	fg := &fakeGuard{}
	rt.SetControlPlaneProbe(ControlPlaneProbe{DB: &fakeCPDB{}, Reconcile: func() reconcile.Stats { return reconcile.Stats{} }, Integrity: fi, Guard: fg})
	call := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := call(http.MethodGet, "/api/v1/system/control-plane-health", "")
	for _, want := range []string{`"db_integrity"`, `"integrity_restore"`, `"reconcile_breakers"`, `"reset_reconcile_guard"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("health is missing %s: %s", want, rec.Body.String())
		}
	}
	if rec := call(http.MethodGet, "/api/v1/system/integrity", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "snap.db") {
		t.Fatalf("GET integrity = %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v1/system/integrity/check", `{"full":true}`); rec.Code != http.StatusOK || len(fi.checks) != 1 || fi.checks[0] != integrity.ModeFull {
		t.Fatalf("check = %d %v", rec.Code, fi.checks)
	}
	if rec := call(http.MethodPost, "/api/v1/system/integrity/restore", `{"name":"snap.db"}`); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "step_up_required") || fi.restored != "" {
		t.Fatalf("a restore without step-up must be refused before anything is staged: %d %s", rec.Code, rec.Body.String())
	}
	stepBody := `{"password":"` + testAdminPassword + `","action":"manage","app":"` + integrityRestoreStepUpApp + `"}`
	if rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/step-up", stepBody)); rec.Code != http.StatusOK {
		t.Fatalf("step-up = %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v1/system/integrity/restore", `{"name":"snap.db"}`); rec.Code != http.StatusAccepted || fi.restored != "snap.db" || !strings.Contains(rec.Body.String(), `"restart"`) {
		t.Fatalf("restore = %d %s", rec.Code, rec.Body.String())
	}
	fi.restore = errors.New("no verified control plane snapshot is available")
	if rec := call(http.MethodPost, "/api/v1/system/integrity/restore", ""); rec.Code != http.StatusConflict {
		t.Fatalf("a refused restore must be 409, got %d", rec.Code)
	}
	fi.restore = cpbackup.ErrInvalidName
	if rec := call(http.MethodPost, "/api/v1/system/integrity/restore", `{"name":"../x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("an invalid name must be 400, got %d", rec.Code)
	}
	if rec := call(http.MethodDelete, "/api/v1/system/integrity/restore", ""); rec.Code != http.StatusNoContent || fi.cancels != 1 {
		t.Fatalf("cancel = %d", rec.Code)
	}
	if rec := call(http.MethodPost, "/api/v1/system/control-plane-health/fix", `{"fix":"reset_reconcile_guard"}`); rec.Code != http.StatusOK || fg.resets != 1 {
		t.Fatalf("guard fix = %d resets %d", rec.Code, fg.resets)
	}
	fi.restore = nil
	if rec := call(http.MethodPost, "/api/v1/system/control-plane-health/fix", `{"fix":"integrity_restore"}`); rec.Code != http.StatusOK {
		t.Fatalf("integrity fix = %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v1/system/control-plane-health/fix", `{"fix":"integrity_check"}`); rec.Code != http.StatusOK {
		t.Fatalf("integrity check fix = %d", rec.Code)
	}
}

func TestIntegrityNotAvailableWithoutProbe(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/integrity", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
}
