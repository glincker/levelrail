package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDeployNotifier is a hand-written fake for DeployNotifier, the same
// pattern internal/webhook/webhook_test.go's own fakeDeployNotifier
// already establishes for the identical interface shape (duplicated
// rather than imported: Go doesn't export test-only types across
// packages, the same reasoning builds_test.go's own fakeBuilder doc
// comment gives for fakeDeployer).
type fakeDeployNotifier struct {
	mu    sync.Mutex
	calls []fakeDeployNotifierCall
}

type fakeDeployNotifierCall struct {
	resourceID string
	ev         alerting.DeployOutcome
}

func (f *fakeDeployNotifier) Dispatch(_ context.Context, resourceID string, ev alerting.DeployOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeDeployNotifierCall{resourceID: resourceID, ev: ev})
}

func (f *fakeDeployNotifier) snapshot() []fakeDeployNotifierCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeDeployNotifierCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// awaitCalls polls snapshot() until it has at least n entries: since
// handleTriggerBuild's build now runs in a background goroutine
// (builds.go), Dispatch can land anytime after the HTTP response
// returns, not necessarily before it.
func (f *fakeDeployNotifier) awaitCalls(t *testing.T, n int) []fakeDeployNotifierCall {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		calls := f.snapshot()
		if len(calls) >= n {
			return calls
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d notifier calls, want at least %d within the deadline", len(calls), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newTestRouterWithDeployNotifyTargets wires notification channels too:
// creating a target now attaches an already-connected channel by ID.
func newTestRouterWithDeployNotifyTargets(t *testing.T) (*Router, *store.DB, *alerting.DB) {
	t.Helper()
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDeployNotifyTargets(adb), WithNotificationChannels(adb))
	return rt, db, adb
}

// seedNotificationChannel saves a ready-to-attach channel directly
// through the store layer, the same split every other seed* helper uses.
func seedNotificationChannel(t *testing.T, adb *alerting.DB, id string, kind alerting.NotifyKind, notifyURL string) {
	t.Helper()
	if err := adb.SaveNotificationChannel(context.Background(), alerting.NotificationChannel{
		ID: id, Name: id, Kind: kind, NotifyURL: notifyURL, Enabled: true,
	}); err != nil {
		t.Fatalf("seed notification channel %q: %v", id, err)
	}
}

func TestDeployNotifyTargetRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDeployNotifyTargets
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	tests := []struct {
		method, target, body string
	}{
		{http.MethodPost, "/api/v1/apps/web/deploy-notify-targets", `{"channel_id":"chn_1","enabled":true}`},
		{http.MethodGet, "/api/v1/apps/web/deploy-notify-targets", ""},
		{http.MethodDelete, "/api/v1/apps/web/deploy-notify-targets/whatever", ""},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tt.method, tt.target, tt.body))
			if rec.Code != http.StatusNotImplemented {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateDeployNotifyTarget_Success(t *testing.T) {
	rt, db, adb := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifySlack, "https://example.com/hook")

	body := `{"channel_id":"chn_1","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploy-notify-targets", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got deployTargetResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a server-generated ID")
	}
	if got.ResourceID != "service:web" {
		t.Errorf("ResourceID = %q, want %q", got.ResourceID, "service:web")
	}
	if got.ChannelID != "chn_1" {
		t.Errorf("ChannelID = %q, want %q", got.ChannelID, "chn_1")
	}
	if got.NotifyURL != "https://example.com/hook" || got.NotifyKind != "slack" || !got.Enabled {
		t.Errorf("got = %+v, want notify_url/notify_kind resolved from the attached channel", got)
	}
}

func TestHandleCreateDeployNotifyTarget_AppNotFound(t *testing.T) {
	rt, db, adb := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifySlack, "https://example.com/hook")

	body := `{"channel_id":"chn_1","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/ghost/deploy-notify-targets", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCreateDeployNotifyTarget_ValidationFailures(t *testing.T) {
	rt, db, adb := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifySlack, "https://example.com/hook")

	tests := []struct {
		name string
		body string
	}{
		{"missing channel_id", `{"enabled":true}`},
		{"unknown channel_id", `{"channel_id":"chn_ghost","enabled":true}`},
		{"malformed body", `{not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploy-notify-targets", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateDeployNotifyTarget_ResourceIDNotCallerSuppliable(t *testing.T) {
	rt, db, adb := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedNotificationChannel(t, adb, "chn_1", alerting.NotifySlack, "https://example.com/hook")

	body := `{"channel_id":"chn_1","enabled":true,"resource_id":"service:someone-elses-app"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploy-notify-targets", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got deployTargetResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ResourceID != "service:web" {
		t.Errorf("ResourceID = %q, want %q (caller-supplied resource_id must be ignored)", got.ResourceID, "service:web")
	}
}

func TestHandleListDeployNotifyTargets(t *testing.T) {
	rt, db, adb := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")

	ctx := context.Background()
	if err := adb.SaveDeployTarget(ctx, alerting.DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://a.example.com", NotifyKind: alerting.NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := adb.SaveDeployTarget(ctx, alerting.DeployTarget{ID: "dnt_2", ResourceID: "service:web", NotifyURL: "https://b.example.com", NotifyKind: alerting.NotifySlack, Enabled: false}); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := adb.SaveDeployTarget(ctx, alerting.DeployTarget{ID: "dnt_3", ResourceID: "service:worker", NotifyURL: "https://c.example.com", NotifyKind: alerting.NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-notify-targets", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []deployTargetResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d targets, want 2 (only service:web's targets, including the disabled one)", len(got))
	}
	for _, target := range got {
		if target.ResourceID != "service:web" {
			t.Errorf("got target %+v scoped to a different resource", target)
		}
	}
}

func TestHandleDeleteDeployNotifyTarget_Success(t *testing.T) {
	rt, db, adb := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	ctx := context.Background()
	if err := adb.SaveDeployTarget(ctx, alerting.DeployTarget{ID: "dnt_1", ResourceID: "service:web", NotifyURL: "https://example.com", NotifyKind: alerting.NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/deploy-notify-targets/dnt_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, err := adb.GetDeployTarget(ctx, "dnt_1"); err == nil {
		t.Error("target still exists after delete")
	}
}

func TestHandleDeleteDeployNotifyTarget_NotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/deploy-notify-targets/nonexistent", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleDeleteDeployNotifyTarget_OwnershipMismatch is the deploy-
// notify-target equivalent of TestHandleDeleteAlertRule_OwnershipMismatch:
// a target belonging to service:worker must not be deletable by guessing
// its ID through service:web's URL.
func TestHandleDeleteDeployNotifyTarget_OwnershipMismatch(t *testing.T) {
	rt, db, adb := newTestRouterWithDeployNotifyTargets(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")

	ctx := context.Background()
	if err := adb.SaveDeployTarget(ctx, alerting.DeployTarget{ID: "dnt_1", ResourceID: "service:worker", NotifyURL: "https://example.com", NotifyKind: alerting.NotifyGeneric, Enabled: true}); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/deploy-notify-targets/dnt_1", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: deleting another app's target through this app's URL must fail as if it didn't exist", rec.Code, http.StatusNotFound)
	}
	if _, err := adb.GetDeployTarget(ctx, "dnt_1"); err != nil {
		t.Errorf("target was deleted despite the ownership mismatch: GetDeployTarget() error = %v", err)
	}
}

func TestDeployNotifyTargetRoutes_RequireAuth(t *testing.T) {
	rt, db, _ := newTestRouterWithDeployNotifyTargets(t)
	seedApp(t, db, "web")

	tests := []struct {
		method, target string
	}{
		{http.MethodPost, "/api/v1/apps/web/deploy-notify-targets"},
		{http.MethodGet, "/api/v1/apps/web/deploy-notify-targets"},
		{http.MethodDelete, "/api/v1/apps/web/deploy-notify-targets/dnt_1"},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

// --- Dispatch wiring: recordPlainDeployAttempt (deploys.go) and
// beginBuildDeployAttempt (deploy_attempts.go) ---

func TestRecordPlainDeployAttempt_DispatchesSucceededNotification(t *testing.T) {
	db := openTestDB(t)
	notifier := &fakeDeployNotifier{}
	rt := NewRouter(nil, testBrand(), db, WithDeployNotifier(notifier))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"levelrail/web:2"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	calls := notifier.snapshot()
	if len(calls) != 1 {
		t.Fatalf("Dispatch called %d times, want 1", len(calls))
	}
	if calls[0].resourceID != "service:web" {
		t.Errorf("resourceID = %q, want %q", calls[0].resourceID, "service:web")
	}
	if calls[0].ev.AppName != "web" || calls[0].ev.Image != "levelrail/web:2" || !calls[0].ev.Succeeded {
		t.Errorf("ev = %+v, want AppName=web Image=levelrail/web:2 Succeeded=true", calls[0].ev)
	}
}

func TestRecordPlainDeployAttempt_NoNotifierConfigured_NoDispatchNoPanic(t *testing.T) {
	// newTestRouter has no WithDeployNotifier: this must behave exactly
	// as it did before deploy-outcome notifications existed, not panic
	// on a nil rt.deployNotifier.
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"levelrail/web:2"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
}

func TestBeginBuildDeployAttempt_DispatchesSucceededNotification(t *testing.T) {
	db := openTestDB(t)
	notifier := &fakeDeployNotifier{}
	rt := NewRouter(nil, testBrand(), db, WithBuilder(newFakeBuilder("levelrail/web:abc123", nil)), WithDeployNotifier(notifier))
	rt.fetch = newFakeFetch("/tmp/checkout-dir", nil).fetch
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"repo_url":"https://example.com/x.git","ref":"main"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	calls := notifier.awaitCalls(t, 1)
	if len(calls) != 1 {
		t.Fatalf("Dispatch called %d times, want 1", len(calls))
	}
	if calls[0].resourceID != "service:web" {
		t.Errorf("resourceID = %q, want %q", calls[0].resourceID, "service:web")
	}
	if !calls[0].ev.Succeeded {
		t.Errorf("ev.Succeeded = false, want true, ev = %+v", calls[0].ev)
	}
}

func TestBeginBuildDeployAttempt_DispatchesFailedNotification(t *testing.T) {
	db := openTestDB(t)
	notifier := &fakeDeployNotifier{}
	rt := NewRouter(nil, testBrand(), db, WithBuilder(newFakeBuilder("", errors.New("buildkit: boom"))), WithDeployNotifier(notifier))
	rt.fetch = newFakeFetch("/tmp/checkout-dir", nil).fetch
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"repo_url":"https://example.com/x.git","ref":"main"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	calls := notifier.awaitCalls(t, 1)
	if len(calls) != 1 {
		t.Fatalf("Dispatch called %d times, want 1", len(calls))
	}
	if calls[0].ev.Succeeded {
		t.Error("ev.Succeeded = true, want false for a failed build")
	}
	if calls[0].ev.Error != "buildkit: boom" {
		t.Errorf("ev.Error = %q, want %q", calls[0].ev.Error, "buildkit: boom")
	}
}
