package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/backup"
	"github.com/glincker/levelrail/internal/cpbackup"
	"github.com/glincker/levelrail/internal/store"
)

type fakeCPDrill struct{ ok bool }

func (f fakeCPDrill) Create(context.Context) (cpbackup.Info, error) { return cpbackup.Info{}, nil }
func (f fakeCPDrill) List() ([]cpbackup.Info, error) {
	return []cpbackup.Info{{Name: "old", CreatedAt: time.Unix(1, 0)}, {Name: "new", CreatedAt: time.Unix(9, 0)}}, nil
}
func (f fakeCPDrill) Open(string) (*os.File, cpbackup.Info, error) { return nil, cpbackup.Info{}, nil }
func (f fakeCPDrill) Delete(string) error                          { return nil }
func (f fakeCPDrill) Verify(_ context.Context, name string) (cpbackup.VerifyResult, error) {
	return cpbackup.VerifyResult{Name: name, OK: f.ok, Checks: []cpbackup.Check{{Name: "sha256", OK: f.ok}}}, nil
}

func TestControlPlaneDrillPassAndFail(t *testing.T) {
	for _, tc := range []struct {
		ok   bool
		want string
	}{{true, "pass"}, {false, "fail"}} {
		rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
		rt.cpBackups = fakeCPDrill{ok: tc.ok}
		cookie := loginTestSession(t, rt, db)
		w := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/drill", `{"control_plane":true}`)
		var got drillLatestResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
			t.Fatalf("code %d body %s", w.Code, w.Body)
		}
		if got.Status != tc.want || got.Kind != "control_plane" {
			t.Fatalf("got %+v", got)
		}
	}
}

func TestDrillLatestNeedsExactlyOneSelector(t *testing.T) {
	rt := &Router{logger: slog.Default()}
	w := httptest.NewRecorder()
	rt.handleBackupDrillLatest(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"app":"a","control_plane":true}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code %d", w.Code)
	}
}

func TestWebDAVTargetValidation(t *testing.T) {
	ctx := context.Background()
	if err := validateWebDAVTarget(ctx, "n", "http://example.com", "d", ""); err == nil {
		t.Fatal("plain http must be refused")
	}
	if err := validateWebDAVTarget(ctx, "n", "https://user:pw@example.com", "d", ""); err == nil {
		t.Fatal("embedded credentials must be refused")
	}
	if err := validateWebDAVTarget(ctx, "n", "https://127.0.0.1", "d", ""); err == nil {
		t.Fatal("loopback must be refused without the private opt-in")
	}
	if err := validateWebDAVTarget(ctx, "n", "https://example.com", "a/../b", ""); err == nil {
		t.Fatal("dotdot refused")
	}
}

func tokenDo(rt *Router, token, body, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, path, body, token))
	return rec
}

func TestControlPlaneDrillNeedsRootAndHidesDetail(t *testing.T) {
	rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
	rt.cpBackups = fakeCPDrill{ok: false}
	ctx := context.Background()
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "ws", Name: "ws", TokenHash: hashToken("tok-ws"), Abilities: []string{AbilityWriteSensitive}}); err != nil {
		t.Fatal(err)
	}
	if rec := tokenDo(rt, "tok-ws", `{"control_plane":true}`, "/api/v1/backups/drill"); rec.Code != http.StatusForbidden {
		t.Fatalf("non-root control plane drill = %d, want 403", rec.Code)
	}
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "root", Name: "root", TokenHash: hashToken("tok-root"), Abilities: []string{AbilityRoot}}); err != nil {
		t.Fatal(err)
	}
	rec := tokenDo(rt, "tok-root", `{"control_plane":true}`, "/api/v1/backups/drill")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "new") {
		t.Fatalf("root drill = %d %s", rec.Code, rec.Body)
	}
}

func TestDrillRoutesHideDeniedApp(t *testing.T) {
	p := &fakeBackupProtection{}
	rt, db := newProtectionRouter(t, p, nil)
	ctx := context.Background()
	seedVolumeBackup(t, db, "ok", store.BackupStatusSucceeded)
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "ws", Name: "ws", TokenHash: hashToken("tok-ws"), Abilities: []string{AbilityWriteSensitive}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePolicy(ctx, store.Policy{ID: "pol_deny", Name: "deny", Document: `{"Statement":[{"Effect":"Deny","Action":["write:sensitive"],"Resource":["app:web"]}]}`}); err != nil {
		t.Fatal(err)
	}
	if err := db.AttachPolicy(ctx, "att_deny", "pol_deny", store.PrincipalTypeToken, "ws"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ path, body string }{
		{"/api/v1/backups/drill", `{"app":"web"}`},
		{"/api/v1/backups/drills", `{"backup_id":"ok"}`},
	} {
		if rec := tokenDo(rt, "tok-ws", c.body, c.path); rec.Code != http.StatusNotFound {
			t.Fatalf("%s denied app = %d %s, want 404", c.path, rec.Code, rec.Body)
		}
	}
	if len(p.drillCalls) != 0 {
		t.Fatalf("a hidden app started a drill: %v", p.drillCalls)
	}
}

func TestDrillStartHidesStatusOfDeniedBackup(t *testing.T) {
	p := &fakeBackupProtection{}
	rt, db := newProtectionRouter(t, p, nil)
	ctx := context.Background()
	seedVolumeBackup(t, db, "bad", store.BackupStatusFailed)
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "ws", Name: "ws", TokenHash: hashToken("tok-ws"), Abilities: []string{AbilityWriteSensitive}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePolicy(ctx, store.Policy{ID: "pol_deny", Name: "deny", Document: `{"Statement":[{"Effect":"Deny","Action":["write:sensitive"],"Resource":["app:web"]}]}`}); err != nil {
		t.Fatal(err)
	}
	if err := db.AttachPolicy(ctx, "att_deny", "pol_deny", store.PrincipalTypeToken, "ws"); err != nil {
		t.Fatal(err)
	}
	if rec := tokenDo(rt, "tok-ws", `{"backup_id":"bad"}`, "/api/v1/backups/drills"); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "status") {
		t.Fatalf("denied backup = %d %s, want a plain 404", rec.Code, rec.Body)
	}
}

func TestDrillStartMapsAdmissionErrors(t *testing.T) {
	for err, want := range map[error]int{backup.ErrDrillBusy: http.StatusConflict, backup.ErrDrillCapacity: http.StatusTooManyRequests} {
		p := &fakeBackupProtection{drillErr: err}
		rt, db := newProtectionRouter(t, p, nil)
		cookie := loginTestSession(t, rt, db)
		seedVolumeBackup(t, db, "ok", store.BackupStatusSucceeded)
		if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/drills", `{"backup_id":"ok"}`); rec.Code != want {
			t.Fatalf("%v = %d, want %d", err, rec.Code, want)
		}
	}
}
