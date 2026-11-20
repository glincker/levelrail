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

	"github.com/GLINCKER/levelrail/internal/cpbackup"
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
		rt := &Router{cpBackups: fakeCPDrill{ok: tc.ok}, logger: slog.Default()}
		w := httptest.NewRecorder()
		rt.handleBackupDrillLatest(w, httptest.NewRequest(http.MethodPost, "/api/v1/backups/drill", strings.NewReader(`{"control_plane":true}`)))
		var got drillLatestResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
			t.Fatalf("code %d body %s", w.Code, w.Body)
		}
		if got.Status != tc.want || got.BackupName != "new" || got.Kind != "control_plane" {
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
