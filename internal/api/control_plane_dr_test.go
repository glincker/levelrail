package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDR struct {
	status   cpbackup.Status
	updateFn func(cpbackup.ConfigUpdate) error
	updated  cpbackup.ConfigUpdate
	escrowFn func(masterKey string, extra []string, upload bool) (cpbackup.EscrowBundle, error)
	started  []string
	busy     bool
	ackErr   error
}

func (f *fakeDR) Status(context.Context) (cpbackup.Status, error) { return f.status, nil }
func (f *fakeDR) UpdateConfig(_ context.Context, u cpbackup.ConfigUpdate) error {
	f.updated = u
	if f.updateFn != nil {
		return f.updateFn(u)
	}
	return nil
}
func (f *fakeDR) ListRemote(context.Context) ([]cpbackup.Remote, error) { return nil, nil }
func (f *fakeDR) StartBackup() error {
	if f.busy {
		return cpbackup.ErrBusy
	}
	f.started = append(f.started, "backup")
	return nil
}
func (f *fakeDR) StartDrill() error {
	f.started = append(f.started, "drill")
	return nil
}
func (f *fakeDR) BuildEscrow(_ context.Context, key string, extra []string, upload bool) (cpbackup.EscrowBundle, error) {
	return f.escrowFn(key, extra, upload)
}
func (f *fakeDR) AckEscrow(context.Context) error { return f.ackErr }

func drRequest(t *testing.T, rt *Router, db *store.DB, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestControlPlaneDR_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	for _, path := range []string{"/api/v1/system/control-plane-dr", "/api/v1/system/control-plane-dr/backups"} {
		if rec := drRequest(t, rt, db, http.MethodGet, path, ""); rec.Code != http.StatusNotImplemented {
			t.Errorf("%s status = %d, want 501", path, rec.Code)
		}
	}
}

func TestControlPlaneDR_StatusSettingsAndRuns(t *testing.T) {
	db := openTestDB(t)
	fake := &fakeDR{status: cpbackup.Status{Enabled: true, Recipients: []string{"age1abc"}, Warnings: []cpbackup.Warning{}}}
	rt := NewRouter(discardLogger(), testBrand(), db, WithControlPlaneDR(fake, nil))

	rec := drRequest(t, rt, db, http.MethodGet, "/api/v1/system/control-plane-dr", "")
	var st cpbackup.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); rec.Code != http.StatusOK || err != nil || !st.Enabled {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	body := `{"enabled":true,"target_id":"t1","recipients":["age1abc"],"schedule":"0 3 * * *","retain_daily":5,"retain_weekly":2,"retain_monthly":1,"escrow_target_id":"t2"}`
	rec = drRequest(t, rt, db, http.MethodPut, "/api/v1/system/control-plane-dr/settings", body)
	if rec.Code != http.StatusOK || fake.updated.TargetID != "t1" || fake.updated.Retention.Daily != 5 || fake.updated.EscrowTargetID != "t2" || fake.updated.Recipients[0] != "age1abc" {
		t.Fatalf("update status = %d updated = %+v", rec.Code, fake.updated)
	}

	fake.updateFn = func(cpbackup.ConfigUpdate) error {
		return errors.Join(cpbackup.ErrInvalid, errors.New("cron expression \"x\""))
	}
	if rec = drRequest(t, rt, db, http.MethodPut, "/api/v1/system/control-plane-dr/settings", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid config status = %d, want 400", rec.Code)
	}
	if rec = drRequest(t, rt, db, http.MethodPut, "/api/v1/system/control-plane-dr/settings", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body status = %d, want 400", rec.Code)
	}

	if rec = drRequest(t, rt, db, http.MethodPost, "/api/v1/system/control-plane-dr/run", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("run status = %d", rec.Code)
	}
	if rec = drRequest(t, rt, db, http.MethodPost, "/api/v1/system/control-plane-dr/drill", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("drill status = %d", rec.Code)
	}
	fake.busy = true
	if rec = drRequest(t, rt, db, http.MethodPost, "/api/v1/system/control-plane-dr/run", ""); rec.Code != http.StatusConflict {
		t.Fatalf("busy run status = %d, want 409", rec.Code)
	}
	if strings.Join(fake.started, ",") != "backup,drill" {
		t.Fatalf("started = %v", fake.started)
	}
}

func TestControlPlaneDR_EscrowNeverLeaksKey(t *testing.T) {
	db := openTestDB(t)
	const secret = "AGE-SECRET-KEY-PQ-1TESTONLY" //nolint:gosec // fake key for a leak check
	var gotKey string
	var gotUpload bool
	fake := &fakeDR{escrowFn: func(k string, _ []string, upload bool) (cpbackup.EscrowBundle, error) {
		gotKey, gotUpload = k, upload
		return cpbackup.EscrowBundle{Armored: "-----BEGIN AGE ENCRYPTED FILE-----", Fingerprint: "abcd", RecipientCount: 1, CreatedAt: time.Now()}, nil
	}}
	rt := NewRouter(discardLogger(), testBrand(), db, WithControlPlaneDR(fake, func() (string, error) { return secret, nil }))

	rec := drRequest(t, rt, db, http.MethodPost, "/api/v1/system/control-plane-dr/escrow", `{"upload":true}`)
	if rec.Code != http.StatusOK || gotKey != secret || !gotUpload {
		t.Fatalf("status = %d key = %q upload = %v", rec.Code, gotKey, gotUpload)
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatal("escrow response contains the plaintext master key")
	}

	fake.escrowFn = func(string, []string, bool) (cpbackup.EscrowBundle, error) {
		return cpbackup.EscrowBundle{}, cpbackup.ErrEscrowSameBucket
	}
	if rec = drRequest(t, rt, db, http.MethodPost, "/api/v1/system/control-plane-dr/escrow", `{"upload":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("same bucket status = %d, want 409", rec.Code)
	}

	rt = NewRouter(discardLogger(), testBrand(), db, WithControlPlaneDR(fake, nil))
	if rec = drRequest(t, rt, db, http.MethodPost, "/api/v1/system/control-plane-dr/escrow", `{}`); rec.Code != http.StatusNotImplemented {
		t.Fatalf("no key reader status = %d, want 501", rec.Code)
	}
}

func TestControlPlaneDR_AckAndDoctor(t *testing.T) {
	db := openTestDB(t)
	fake := &fakeDR{ackErr: errors.New("generate an escrow bundle before acknowledging it")}
	rt := NewRouter(discardLogger(), testBrand(), db, WithControlPlaneDR(fake, nil))
	if rec := drRequest(t, rt, db, http.MethodPost, "/api/v1/system/control-plane-dr/escrow/ack", ""); rec.Code != http.StatusConflict {
		t.Fatalf("ack before escrow status = %d, want 409", rec.Code)
	}

	ctx := context.Background()
	if c := rt.doctorCheckControlPlaneDR(ctx); c.Status != doctorStatusWarn || c.Fix == "" {
		t.Fatalf("disabled DR check = %+v", c)
	}
	fake.status = cpbackup.Status{Enabled: true, Warnings: []cpbackup.Warning{{Code: "no_drill", Message: "No restore drill has run yet."}}}
	if c := rt.doctorCheckControlPlaneDR(ctx); c.Status != doctorStatusWarn || c.Message != "No restore drill has run yet." {
		t.Fatalf("warning DR check = %+v", c)
	}
	fake.status = cpbackup.Status{Enabled: true, Warnings: []cpbackup.Warning{}}
	if c := rt.doctorCheckControlPlaneDR(ctx); c.Status != doctorStatusOK {
		t.Fatalf("healthy DR check = %+v", c)
	}
}
