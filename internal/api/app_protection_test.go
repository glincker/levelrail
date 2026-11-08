package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeProtectApps struct {
	AppStore
	p *store.AppProtection
}

func (f *fakeProtectApps) GetAppProtection(context.Context, string) (*store.AppProtection, error) {
	return f.p, nil
}
func (f *fakeProtectApps) SetAppProtection(context.Context, string, string, string) error { return nil }
func (f *fakeProtectApps) ClearAppProtection(context.Context, string) (bool, error)       { return true, nil }

func TestAppLockRefusesWhenProtected(t *testing.T) {
	rt := &Router{apps: &fakeProtectApps{p: &store.AppProtection{Reason: "demo week"}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/web", nil)
	if !rt.refuseIfProtected(w, r, "web") {
		t.Fatal("protected app was not refused")
	}
	if w.Code != http.StatusLocked || !strings.Contains(w.Body.String(), "demo week") {
		t.Fatalf("got %d %s, want 423 with the reason", w.Code, w.Body.String())
	}
}

func TestAppLockAllowsWhenNotProtected(t *testing.T) {
	rt := &Router{apps: &fakeProtectApps{}}
	w := httptest.NewRecorder()
	if rt.refuseIfProtected(w, httptest.NewRequest(http.MethodPut, "/x", nil), "web") {
		t.Fatalf("unprotected app refused: %d", w.Code)
	}
}

func TestAppLockPutRequiresReason(t *testing.T) {
	rt := &Router{apps: &fakeProtectApps{}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/protection", strings.NewReader(`{"reason":"  "}`))
	r.SetPathValue("name", "web")
	rt.handlePutAppProtection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
}
